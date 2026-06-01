// Package scheduler provides automated report scheduling functionality for Repo-lyzer.
// It enables periodic analysis reports that run automatically and export results
// to various formats (JSON, PDF, Markdown) and destinations (local path, webhook).
package scheduler

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/agnivo988/Repo-lyzer/internal/analyzer"
    "github.com/agnivo988/Repo-lyzer/internal/config"
    "github.com/agnivo988/Repo-lyzer/internal/github"
    "github.com/agnivo988/Repo-lyzer/internal/output"
    "github.com/agnivo988/Repo-lyzer/internal/monitor"
    "github.com/robfig/cron/v3"
    "golang.org/x/sync/semaphore"
)

// Scheduler manages scheduled analysis report jobs.
type Scheduler struct {
    cron           *cron.Cron
    settings       *config.AppSettings
    jobEntries     map[string]cron.EntryID
    reportExporter *ReportExporter
    jobCooldowns   map[string]time.Time // tracks last execution time for cooldown
    maxWorkers     int
    workerSem      *semaphore.Weighted
    // Fair queue fields
    pendingJobs    []config.ScheduledJob
    queueMutex     sync.Mutex
    workerFairness int // round‑robin index
    // Starvation prevention telemetry
    starvationCount     int
    starvationThreshold time.Duration
    // Worker timeout configuration
    workerExecutionTimeout time.Duration
    ctx                    context.Context
    cancel                 context.CancelFunc
    stopChan       chan struct{}
    wg             sync.WaitGroup
}

// ReportExporter handles exporting analysis reports to various formats.
type ReportExporter struct{}

// NewScheduler creates a new scheduler instance.
func NewScheduler() (*Scheduler, error) {
    settings, err := config.LoadSettings()
    if err != nil {
        return nil, fmt.Errorf("failed to load settings: %w", err)
    }

    // Default max workers; can be made configurable later.
    maxWorkers := 5
    ctx, cancel := context.WithCancel(context.Background())

    return &Scheduler{
        cron:           cron.New(),
        settings:       settings,
        jobEntries:     make(map[string]cron.EntryID),
        reportExporter: &ReportExporter{},
        jobCooldowns:   make(map[string]time.Time),
        maxWorkers:     maxWorkers,
        workerSem:      semaphore.NewWeighted(int64(maxWorkers)),
        pendingJobs:    make([]config.ScheduledJob, 0),
        queueMutex:     sync.Mutex{},
        workerFairness: 0,
        starvationCount:     0,
        starvationThreshold: 2 * time.Minute,
        // Align with design: 2‑minute timeout.
        workerExecutionTimeout: 2 * time.Minute,
        ctx:            ctx,
        cancel:         cancel,
        stopChan:       make(chan struct{}),
        wg:             sync.WaitGroup{},
    }, nil
}

// Start initializes and starts the scheduler with all registered jobs.
func (s *Scheduler) Start() error {
    log.Println("Starting scheduler...")

    jobs := s.settings.GetScheduledJobs()
    for _, job := range jobs {
        if !job.Enabled {
            continue
        }
        if cd, ok := s.jobCooldowns[job.ID]; ok && time.Now().Before(cd) {
            log.Printf("Job %s is in cooldown until %s, skipping schedule", job.ID, cd.Format(time.RFC3339))
            continue
        }
        if err := s.scheduleJob(job); err != nil {
            log.Printf("Failed to schedule job %s: %v", job.ID, err)
        }
    }

    s.wg.Add(1)
    go s.processQueue()

    s.cron.Start()
    log.Printf("Scheduler started with %d enabled jobs", len(jobs))
    return nil
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
    log.Println("Stopping scheduler...")
    s.cron.Stop()
    close(s.stopChan)
    s.wg.Wait()
    log.Println("Scheduler stopped")
}

// scheduleJob adds a job to the cron scheduler.
func (s *Scheduler) scheduleJob(job config.ScheduledJob) error {
    spec := job.GetCronExpression()
    jobFunc := func() {
        log.Printf("Enqueuing scheduled job: %s for %s/%s", job.ID, job.Owner, job.Repo)
        if cd, ok := s.jobCooldowns[job.ID]; ok && time.Now().Before(cd) {
            log.Printf("Job %s is in cooldown until %s, skipping enqueue", job.ID, cd.Format(time.RFC3339))
            return
        }
        s.queueMutex.Lock()
        job.EnqueuedAt = time.Now()
        s.pendingJobs = append(s.pendingJobs, job)
        s.queueMutex.Unlock()
    }
    entryID, err := s.cron.AddFunc(spec, jobFunc)
    if err != nil {
        return fmt.Errorf("failed to add cron job: %w", err)
    }
    s.jobEntries[job.ID] = entryID
    log.Printf("Job %s scheduled with spec: %s", job.ID, spec)
    return nil
}

// executeJob runs the analysis and exports the report.
// It assumes the caller has already acquired a worker slot.
func (s *Scheduler) executeJob(job config.ScheduledJob) error {
    startTime := time.Now()

    client := github.NewClient()
    repoInfo, err := client.GetRepo(job.Owner, job.Repo)
    if err != nil {
        return fmt.Errorf("failed to get repository: %w", err)
    }
    langs, err := client.GetLanguages(job.Owner, job.Repo)
    if err != nil {
        return fmt.Errorf("failed to get languages: %w", err)
    }
    commits, err := client.GetCommits(job.Owner, job.Repo, 365)
    if err != nil {
        return fmt.Errorf("failed to get commits: %w", err)
    }
    contributors, err := client.GetContributors(job.Owner, job.Repo)
    if err != nil {
        return fmt.Errorf("failed to get contributors: %w", err)
    }

    healthScore := analyzer.CalculateHealth(repoInfo, commits)
    busFactor, busRisk := analyzer.BusFactor(contributors)
    maturityScore, maturityLevel := analyzer.RepoMaturityScore(repoInfo, len(commits), len(contributors), false)

    compactCfg := output.CompactConfig{
        Repo:            repoInfo,
        HealthScore:     healthScore,
        BusFactor:       busFactor,
        BusRisk:         busRisk,
        MaturityScore:   maturityScore,
        MaturityLevel:   maturityLevel,
        CommitsLastYear: len(commits),
        Contributors:    len(contributors),
        Duration:        time.Since(startTime),
        Languages:       langs,
    }

    var reportData []byte
    var filename string
    switch job.Format {
    case config.ExportJSON:
        reportData, err = s.reportExporter.exportJSON(compactCfg)
        filename = fmt.Sprintf("%s_%s_report.json", job.Owner+"-"+job.Repo, time.Now().Format("20060102"))
    case config.ExportMarkdown:
        reportData, err = s.reportExporter.exportMarkdown(compactCfg, repoInfo)
        filename = fmt.Sprintf("%s_%s_report.md", job.Owner+"-"+job.Repo, time.Now().Format("20060102"))
    case config.ExportPDF:
        reportData, err = s.reportExporter.exportJSON(compactCfg)
        filename = fmt.Sprintf("%s_%s_report.json", job.Owner+"-"+job.Repo, time.Now().Format("20060102"))
        log.Println("PDF export not fully implemented, exporting as JSON instead")
    default:
        reportData, err = s.reportExporter.exportJSON(compactCfg)
        filename = fmt.Sprintf("%s_%s_report.json", job.Owner+"-"+job.Repo, time.Now().Format("20060102"))
    }
    if err != nil {
        return fmt.Errorf("failed to export report: %w", err)
    }

    if err := s.saveReport(job.Destination, filename, reportData); err != nil {
        return fmt.Errorf("failed to save report: %w", err)
    }

    // Update job metadata.
    job.LastRun = time.Now()
    job.NextRun = s.calculateNextRunTime(job.GetCronExpression())
    s.settings.UpdateScheduledJob(job)

    // Apply a simple 1‑minute cooldown to avoid immediate re‑run.
    s.jobCooldowns[job.ID] = time.Now().Add(time.Minute)
    log.Printf("Job %s completed successfully (cooldown applied)", job.ID)
    return nil
}

// exportJSON exports the report as JSON.
func (s *ReportExporter) exportJSON(cfg output.CompactConfig) ([]byte, error) {
    var buf bytes.Buffer
    encoder := json.NewEncoder(&buf)
    encoder.SetEscapeHTML(false)
    if err := encoder.Encode(cfg); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// exportMarkdown exports the report as Markdown.
func (s *ReportExporter) exportMarkdown(cfg output.CompactConfig, repoInfo *github.Repo) ([]byte, error) {
    var buf bytes.Buffer
    buf.WriteString("# Repository Analysis Report\n\n")
    buf.WriteString(fmt.Sprintf("**Repository:** %s\n\n", cfg.Repo.FullName))
    buf.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
    buf.WriteString("## Metrics Summary\n\n")
    buf.WriteString(fmt.Sprintf("- **Health Score:** %d/100\n", cfg.HealthScore))
    buf.WriteString(fmt.Sprintf("- **Bus Factor:** %d\n", cfg.BusFactor))
    buf.WriteString(fmt.Sprintf("- **Bus Risk:** %s\n", cfg.BusRisk))
    buf.WriteString(fmt.Sprintf("- **Maturity Score:** %d\n", cfg.MaturityScore))
    buf.WriteString(fmt.Sprintf("- **Maturity Level:** %s\n", cfg.MaturityLevel))
    buf.WriteString(fmt.Sprintf("- **Commits (1 year):** %d\n", cfg.CommitsLastYear))
    buf.WriteString(fmt.Sprintf("- **Contributors:** %d\n\n", cfg.Contributors))
    buf.WriteString("## Repository Info\n\n")
    buf.WriteString(fmt.Sprintf("- **Stars:** %d\n", cfg.Repo.Stars))
    buf.WriteString(fmt.Sprintf("- **Forks:** %d\n", cfg.Repo.Forks))
    buf.WriteString(fmt.Sprintf("- **Open Issues:** %d\n", cfg.Repo.OpenIssues))
    if cfg.Repo.Language != "" {
        buf.WriteString(fmt.Sprintf("- **Primary Language:** %s\n", cfg.Repo.Language))
    }
    if cfg.Repo.Description != "" {
        buf.WriteString(fmt.Sprintf("\n**Description:** %s\n", cfg.Repo.Description))
    }
    return buf.Bytes(), nil
}

// saveReport saves the report to the specified destination.
func (s *Scheduler) saveReport(dest config.OutputDestination, filename string, data []byte) error {
    if !dest.Enabled {
        return fmt.Errorf("destination is not enabled")
    }
    switch dest.Type {
    case "local":
        return s.saveToLocalPath(dest.LocalPath, filename, data)
    case "webhook":
        return s.sendToWebhook(dest.WebhookURL, filename, data)
    default:
        return s.saveToLocalPath(dest.LocalPath, filename, data)
    }
}

func (s *Scheduler) saveToLocalPath(localPath, filename string, data []byte) error {
    if localPath == "" {
        localPath = s.settings.ExportDirectory
    }
    if err := os.MkdirAll(localPath, 0755); err != nil {
        return fmt.Errorf("failed to create directory: %w", err)
    }
    filePath := filepath.Join(localPath, filename)
    if err := os.WriteFile(filePath, data, 0644); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }
    log.Printf("Report saved to: %s", filePath)
    return nil
}

func (s *Scheduler) sendToWebhook(webhookURL, filename string, data []byte) error {
    if webhookURL == "" {
        return fmt.Errorf("webhook URL is not configured")
    }
    payload := map[string]interface{}{
        "filename":  filename,
        "content":   string(data),
        "timestamp": time.Now().Format(time.RFC3339),
    }
    jsonData, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("failed to marshal payload: %w", err)
    }
    resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("failed to send webhook: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        return fmt.Errorf("webhook returned status: %d", resp.StatusCode)
    }
    log.Printf("Report sent to webhook: %s", webhookURL)
    return nil
}

// AddJob adds a new scheduled job.
func (s *Scheduler) AddJob(job config.ScheduledJob) error {
    if err := s.settings.AddScheduledJob(job); err != nil {
        return fmt.Errorf("failed to add job: %w", err)
    }
    if job.Enabled {
        return s.scheduleJob(job)
    }
    return nil
}

// RemoveJob removes a scheduled job.
func (s *Scheduler) RemoveJob(jobID string) error {
    if entryID, ok := s.jobEntries[jobID]; ok {
        s.cron.Remove(entryID)
        delete(s.jobEntries, jobID)
    }
    return s.settings.RemoveScheduledJob(jobID)
}

// ListJobs returns all scheduled jobs.
func (s *Scheduler) ListJobs() []config.ScheduledJob {
    return s.settings.GetScheduledJobs()
}

// GetJob returns a specific job by ID.
func (s *Scheduler) GetJob(jobID string) *config.ScheduledJob {
    return s.settings.GetScheduledJobByID(jobID)
}

// EnableJob enables or disables a job.
func (s *Scheduler) EnableJob(jobID string, enabled bool) error {
    job := s.settings.GetScheduledJobByID(jobID)
    if job == nil {
        return fmt.Errorf("job not found: %s", jobID)
    }
    job.Enabled = enabled
    if enabled {
        if err := s.scheduleJob(*job); err != nil {
            return fmt.Errorf("failed to schedule job: %w", err)
        }
    } else {
        if entryID, ok := s.jobEntries[jobID]; ok {
            s.cron.Remove(entryID)
            delete(s.jobEntries, jobID)
        }
    }
    return s.settings.EnableScheduledJob(jobID, enabled)
}

// calculateNextRunTime calculates the next run time based on cron expression.
func (s *Scheduler) calculateNextRunTime(cronExpr string) time.Time {
    parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
    sched, err := parser.Parse(cronExpr)
    if err != nil {
        return time.Now().Add(24 * time.Hour)
    }
    return sched.Next(time.Now())
}

func ValidateCronExpression(expr string) error {
    parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
    _, err := parser.Parse(expr)
    return err
}

func FormatScheduleInterval() string {
    var intervals []string
    intervals = append(intervals, string(config.ScheduleDaily))
    intervals = append(intervals, string(config.ScheduleWeekly))
    intervals = append(intervals, string(config.ScheduleMonthly))
    intervals = append(intervals, string(config.ScheduleCustom))
    return strings.Join(intervals, ", ")
}

func GetCronExpressionForInterval(interval config.ScheduleInterval) string { return interval.CronExpression() }

// processQueue runs a simple FIFO fair queue processor.
func (s *Scheduler) processQueue() {
    defer s.wg.Done()
    for {
        select {
        case <-s.stopChan:
            return
        default:
            var job config.ScheduledJob
            s.queueMutex.Lock()
            if len(s.pendingJobs) > 0 {
                // Pick the oldest enqueued job.
                oldestIdx := 0
                oldestTime := s.pendingJobs[0].EnqueuedAt
                for i, j := range s.pendingJobs {
                    if j.EnqueuedAt.Before(oldestTime) {
                        oldestIdx = i
                        oldestTime = j.EnqueuedAt
                    }
                }
                job = s.pendingJobs[oldestIdx]
                s.pendingJobs = append(s.pendingJobs[:oldestIdx], s.pendingJobs[oldestIdx+1:]...)
                // Starvation telemetry.
                if s.starvationThreshold > 0 && time.Since(job.EnqueuedAt) > s.starvationThreshold {
                    s.starvationCount++
                    log.Printf("Starvation detected for job %s (waited %s)", job.ID, time.Since(job.EnqueuedAt))
                }
            }
            s.queueMutex.Unlock()
            if job.ID == "" {
                time.Sleep(100 * time.Millisecond)
                continue
            }
            s.wg.Add(1)
            go func(j config.ScheduledJob) {
                defer s.wg.Done()
                ctx, cancel := context.WithTimeout(s.ctx, s.workerExecutionTimeout)
                defer cancel()
                if err := s.runJobWithTimeout(ctx, j); err != nil {
                    if errors.Is(err, context.DeadlineExceeded) {
                        log.Printf("scheduler worker timeout: job=%s timeout=%v", j.ID, s.workerExecutionTimeout)
                    } else {
                        log.Printf("error executing job %s: %v", j.ID, err)
                    }
                }
            }(job)
        }
    }
}

// runJobWithTimeout is a thin wrapper that simply calls executeJob.
// The semaphore acquire is performed in runJobWithTimeout to keep a single ownership point.
func (s *Scheduler) runJobWithTimeout(ctx context.Context, job config.ScheduledJob) error {
    if err := s.workerSem.Acquire(ctx, 1); err != nil {
        return err
    }
    defer s.workerSem.Release(1)
    return s.executeJob(job)
}
