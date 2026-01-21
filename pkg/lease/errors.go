package lease

import "errors"

// LeaseHeldError indicates a lease could not be acquired because it is held by another contender.
// This is the "expected" failure mode for lock contention.
type LeaseHeldError struct {
	Key Key
}

func (e *LeaseHeldError) Error() string {
	return "lease held"
}

// LeaseNotOwnedError indicates a refresh attempt failed because the caller's token no longer owns the lease.
// This can happen if the lease expired and was acquired by another contender, or if the token is wrong.
type LeaseNotOwnedError struct {
	Key Key
}

func (e *LeaseNotOwnedError) Error() string {
	return "lease not owned"
}

func IsLeaseHeld(err error) bool {
	var target *LeaseHeldError
	return errors.As(err, &target)
}

func IsLeaseNotOwned(err error) bool {
	var target *LeaseNotOwnedError
	return errors.As(err, &target)
}
