package network

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR} -lnetwork
#include <stdlib.h>
#include <stddef.h>
#include <stdint.h>
#include "network.h"
*/
import "C"

import "errors"

// Init initializes the C networking library (no-op on Linux).
// This MUST be called once at the start of the application.
func Init() error {
	if C.network_init() != 0 {
		return errors.New("failed to initialize C networking library")
	}
	return nil
}

// Cleanup cleans up the C networking library (no-op on Linux).
// This MUST be called once before the application exits.
func Cleanup() {
	C.network_cleanup()
}
