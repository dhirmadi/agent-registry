package telemetry

import "context"

// Shutdown is a function that cleans up telemetry resources.
type Shutdown func(ctx context.Context) error

// Init initializes OpenTelemetry providers.
// This is a stub — real OTel setup will be added in a later phase.
func Init(ctx context.Context, serviceName, endpoint string) (Shutdown, error) {
	return func(ctx context.Context) error {
		return nil
	}, nil
}
