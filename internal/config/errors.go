package config

import "errors"

// Sentinel errors for the config package.
var (
	ErrCircularReference  = errors.New("circular reference")
	ErrUndefinedEnvVar    = errors.New("undefined environment variable")
	ErrInvalidEnvSyntax   = errors.New("invalid env syntax")
	ErrWorkdirConflict    = errors.New("workdir conflict")
	ErrInvalidMountFormat = errors.New("invalid mount format")
	ErrInvalidMountOption = errors.New("invalid mount option")
	ErrMountSourceEmpty   = errors.New("mount source empty")
	ErrMountTargetEmpty   = errors.New("mount target empty")
	ErrInvalidType        = errors.New("invalid type")
	ErrUnknownAlcaToken   = errors.New("unknown alca token")
	ErrInvalidAlcaToken   = errors.New("invalid alca token")
)
