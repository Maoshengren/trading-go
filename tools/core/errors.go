package core

import "errors"

var (
	ErrInsufficientBars      = errors.New("not enough kline data")
	ErrProviderNotConfigured = errors.New("market data provider is not configured")
	ErrFeatureNotImplemented = errors.New("market data feature is not implemented for the current provider")
)
