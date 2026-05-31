package engine

import (
    "context"
    "fmt"
)

// recoverPanic recovers from a panic and returns an error with stack trace.
func recoverPanic(name string) (err error) {
    if r := recover(); r != nil {
        // Capture stack trace (optional, for now just format)
        err = fmt.Errorf("panic in %s: %v", name, r)
    }
    return err
}
