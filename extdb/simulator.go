package extdb

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// SimulatorOpts configures the database simulator behavior.
type SimulatorOpts struct {
	// Latency is the encoded latency and probability in format: <probability>%<duration>,<probability>%<duration>...
	// e.g., "90%10ms,10%100ms" means 90% of queries take 10ms, 10% take 100ms.
	Latency string

	// SuccessProb is the probability (in %) of a successful query (0-100).
	SuccessProb float64

	// ErrorTypes defines the distribution of error types when errors occur.
	// Format: <probability>%<error_type>,<probability>%<error_type>...
	// e.g., "50%timeout,30%connection,20%deadlock"
	// If not set, defaults to "100%generic"
	ErrorTypes string
}

// DefaultSimulatorOpts returns default simulator options.
func DefaultSimulatorOpts() SimulatorOpts {
	return SimulatorOpts{
		Latency:     "90%10ms,10%50ms",
		SuccessProb: 95,
		ErrorTypes:  "50%timeout,30%connection,20%deadlock",
	}
}

// Simulator simulates database operations with configurable latency and errors.
type Simulator struct {
	metrics      *Metrics
	latDecider   *latencyDecider
	errorDecider *errorDecider
	successProb  float64
}

// NewSimulator creates a new database simulator.
func NewSimulator(metrics *Metrics, opts SimulatorOpts) (*Simulator, error) {
	latDecider, err := newLatencyDecider(opts.Latency)
	if err != nil {
		return nil, errors.Wrap(err, "parsing latency")
	}

	errorTypes := opts.ErrorTypes
	if errorTypes == "" {
		errorTypes = "100%generic"
	}
	errorDecider, err := newErrorDecider(errorTypes)
	if err != nil {
		return nil, errors.Wrap(err, "parsing error types")
	}

	return &Simulator{
		metrics:      metrics,
		latDecider:   latDecider,
		errorDecider: errorDecider,
		successProb:  opts.SuccessProb,
	}, nil
}

// QueryResult represents the result of a simulated query.
type QueryResult struct {
	Success      bool
	ErrorType    string
	Duration     time.Duration
	RowsAffected int
}

// SimulateQuery simulates a database query with the configured latency and error rates.
// It records metrics and returns the result.
func (s *Simulator) SimulateQuery(ctx context.Context, operation, table string) QueryResult {
	s.metrics.IncInflight(operation, table)
	defer s.metrics.DecInflight(operation, table)

	start := time.Now()

	// Add latency
	latency := s.latDecider.GetLatency()

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		s.metrics.RecordError(operation, table, "context_cancelled")
		s.metrics.RecordQuery(operation, table, "error", time.Since(start).Seconds())
		return QueryResult{
			Success:   false,
			ErrorType: "context_cancelled",
			Duration:  time.Since(start),
		}
	default:
	}

	// Wait for latency or context cancellation
	select {
	case <-ctx.Done():
		s.metrics.RecordError(operation, table, "context_cancelled")
		s.metrics.RecordQuery(operation, table, "error", time.Since(start).Seconds())
		return QueryResult{
			Success:   false,
			ErrorType: "context_cancelled",
			Duration:  time.Since(start),
		}
	case <-time.After(latency):
	}

	duration := time.Since(start)

	// Determine success or failure
	if rand.Float64()*100 <= s.successProb {
		rowsAffected := rand.Intn(100) + 1 // Random rows affected between 1-100
		s.metrics.RecordQuery(operation, table, "success", duration.Seconds())
		s.metrics.RecordRowsAffected(operation, table, float64(rowsAffected))

		slog.Debug("simulated db query succeeded",
			"operation", operation,
			"table", table,
			"duration", duration,
			"rows_affected", rowsAffected,
		)

		return QueryResult{
			Success:      true,
			Duration:     duration,
			RowsAffected: rowsAffected,
		}
	}

	// Query failed
	errorType := s.errorDecider.GetErrorType()
	s.metrics.RecordQuery(operation, table, "error", duration.Seconds())
	s.metrics.RecordError(operation, table, errorType)

	slog.Warn("simulated db query failed",
		"operation", operation,
		"table", table,
		"duration", duration,
		"error_type", errorType,
	)

	return QueryResult{
		Success:   false,
		ErrorType: errorType,
		Duration:  duration,
	}
}

// SimulateSelect simulates a SELECT query.
func (s *Simulator) SimulateSelect(ctx context.Context, table string) QueryResult {
	return s.SimulateQuery(ctx, "select", table)
}

// SimulateInsert simulates an INSERT query.
func (s *Simulator) SimulateInsert(ctx context.Context, table string) QueryResult {
	return s.SimulateQuery(ctx, "insert", table)
}

// SimulateUpdate simulates an UPDATE query.
func (s *Simulator) SimulateUpdate(ctx context.Context, table string) QueryResult {
	return s.SimulateQuery(ctx, "update", table)
}

// SimulateDelete simulates a DELETE query.
func (s *Simulator) SimulateDelete(ctx context.Context, table string) QueryResult {
	return s.SimulateQuery(ctx, "delete", table)
}

// latencyDecider determines latency based on configured probabilities.
type latencyDecider struct {
	latencies     []time.Duration
	probabilities []float64
}

func newLatencyDecider(encodedLatencies string) (*latencyDecider, error) {
	l := latencyDecider{}

	s := strings.Split(encodedLatencies, ",")
	sort.Strings(s)

	cumulativeProb := 0.0
	for _, e := range s {
		entry := strings.Split(e, "%")
		if len(entry) != 2 {
			return nil, errors.Errorf("invalid latency input %v", encodedLatencies)
		}
		f, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parse probability %v as float", entry[0])
		}
		cumulativeProb += f
		l.probabilities = append(l.probabilities, f)

		d, err := time.ParseDuration(entry[1])
		if err != nil {
			return nil, errors.Wrapf(err, "parse latency %v as duration", entry[1])
		}
		l.latencies = append(l.latencies, d)
	}
	if cumulativeProb != 100 {
		return nil, errors.Errorf("overall probability has to equal 100. Parsed input equals to %v", cumulativeProb)
	}
	slog.Info("db latency decider created", "latencies", l.latencies, "probabilities", l.probabilities)
	return &l, nil
}

func (l *latencyDecider) GetLatency() time.Duration {
	n := rand.Float64() * 100
	for i, p := range l.probabilities {
		if n <= p {
			return l.latencies[i]
		}
	}
	return l.latencies[len(l.latencies)-1]
}

// errorDecider determines error type based on configured probabilities.
type errorDecider struct {
	errorTypes    []string
	probabilities []float64
}

func newErrorDecider(encodedErrors string) (*errorDecider, error) {
	e := errorDecider{}

	s := strings.Split(encodedErrors, ",")
	sort.Strings(s)

	cumulativeProb := 0.0
	for _, entry := range s {
		parts := strings.Split(entry, "%")
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid error type input %v", encodedErrors)
		}
		f, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parse probability %v as float", parts[0])
		}
		cumulativeProb += f
		e.probabilities = append(e.probabilities, f)
		e.errorTypes = append(e.errorTypes, parts[1])
	}
	if cumulativeProb != 100 {
		return nil, errors.Errorf("overall error probability has to equal 100. Parsed input equals to %v", cumulativeProb)
	}
	slog.Info("db error decider created", "error_types", e.errorTypes, "probabilities", e.probabilities)
	return &e, nil
}

func (e *errorDecider) GetErrorType() string {
	n := rand.Float64() * 100
	for i, p := range e.probabilities {
		if n <= p {
			return e.errorTypes[i]
		}
	}
	return e.errorTypes[len(e.errorTypes)-1]
}

// SimulatedError represents a simulated database error.
type SimulatedError struct {
	Type    string
	Message string
}

func (e SimulatedError) Error() string {
	return fmt.Sprintf("simulated db error [%s]: %s", e.Type, e.Message)
}
