// Adapted from https://github.com/AnaisUrlichs/observe-argo-rollout/tree/main/app

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	psflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/saswatamcode/pingpong/extdb"
	"github.com/saswatamcode/pingpong/exthttp"
	"github.com/spf13/cobra"
)

var (
	latDecider  *latencyDecider
	dbSimulator *extdb.Simulator

	// root command flags
	logLevelStr  string
	logFormatStr string

	// pong command flags
	pongAddr    string
	appVersion  string
	lat         string
	successProb float64

	// database simulation flags
	dbEnabled     bool
	dbLatency     string
	dbSuccessProb float64
	dbErrorTypes  string

	// ping command flags
	pingAddr    string
	endpoint    string
	pingsPerSec int
)

var rootCmd = &cobra.Command{
	Use:   "pingpong",
	Short: "Pingpong is a demo HTTP client/server for testing",
	Long:  "Pingpong provides ping and pong commands for testing HTTP request/response with configurable latency and metrics.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		logLevel := promslog.NewLevel()
		if err := logLevel.Set(logLevelStr); err != nil {
			panic(fmt.Sprintf("invalid log level: %v", err))
		}

		logFormat := promslog.NewFormat()
		if err := logFormat.Set(logFormatStr); err != nil {
			panic(fmt.Sprintf("invalid log format: %v", err))
		}

		slog.SetDefault(promslog.New(&promslog.Config{
			Level:  logLevel,
			Format: logFormat,
			Style:  promslog.GoKitStyle,
			Writer: os.Stderr,
		}))

		return nil
	},
}

var pongCmd = &cobra.Command{
	Use:   "pong",
	Short: "Start the pong HTTP server",
	Long:  "Start the pong HTTP server that responds to /ping requests with configurable latency and success probability.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPongServer()
	},
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Start the ping client that sends requests to a pong server",
	Long:  "Start the ping client that continuously sends HTTP requests to a pong server endpoint with configurable rate.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPinger()
	},
}

func init() {
	// root command flags
	rootCmd.PersistentFlags().StringVar(&logLevelStr, psflag.LevelFlagName, "info", psflag.LevelFlagHelp)
	rootCmd.PersistentFlags().StringVar(&logFormatStr, psflag.FormatFlagName, "logfmt", psflag.FormatFlagHelp)

	// pong command flags
	pongCmd.Flags().StringVar(&pongAddr, "listen-address", ":8080", "The address to listen on for HTTP requests.")
	pongCmd.Flags().StringVar(&appVersion, "set-version", "first", "Injected version to be presented via metrics.")
	pongCmd.Flags().StringVar(&lat, "latency", "90%500ms,10%200ms", "Encoded latency and probability of the response in format as: <probability>%<duration>,<probability>%<duration>....")
	pongCmd.Flags().Float64Var(&successProb, "success-prob", 100, "The probability (in %) of getting a successful response")

	// database simulation flags
	pongCmd.Flags().BoolVar(&dbEnabled, "db-enabled", false, "Enable database simulation metrics")
	pongCmd.Flags().StringVar(&dbLatency, "db-latency", "90%10ms,10%50ms", "Encoded latency and probability for simulated DB queries in format: <probability>%<duration>,<probability>%<duration>....")
	pongCmd.Flags().Float64Var(&dbSuccessProb, "db-success-prob", 95, "The probability (in %) of a successful simulated DB query")
	pongCmd.Flags().StringVar(&dbErrorTypes, "db-error-types", "50%timeout,30%connection,20%deadlock", "Distribution of error types when DB queries fail in format: <probability>%<error_type>,...")

	// ping command flags
	pingCmd.Flags().StringVar(&pingAddr, "listen-address", ":8080", "The address to listen on for HTTP requests.")
	pingCmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8080/ping", "The address of pong app we can connect to and send requests.")
	pingCmd.Flags().IntVar(&pingsPerSec, "pings-per-second", 10, "How many pings per second we should request")

	rootCmd.AddCommand(pongCmd)
	rootCmd.AddCommand(pingCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Debug("command execution failed", "error", err)
		os.Exit(1)
	}
}

type latencyDecider struct {
	latencies     []time.Duration
	probabilities []float64 // Sorted ascending.
}

func newLatencyDecider(encodedLatencies string) (*latencyDecider, error) {
	l := latencyDecider{}

	s := strings.Split(encodedLatencies, ",")
	sort.Strings(s)

	cumulativeProb := 0.0
	for _, e := range s {
		entry := strings.Split(e, "%")
		if len(entry) != 2 {
			return nil, errors.Errorf("invalid input %v", encodedLatencies)
		}
		f, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parse probabilty %v as float", entry[0])
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
	slog.Info("latency decider created", "latencies", l.latencies, "probabilities", l.probabilities)
	return &l, nil
}

func (l latencyDecider) AddLatency(ctx context.Context) {
	n := rand.Float64() * 100
	for i, p := range l.probabilities {
		if n <= p {
			<-time.After(l.latencies[i])
			return
		}
	}
}

func handlerPing(w http.ResponseWriter, r *http.Request) {
	latDecider.AddLatency(r.Context())

	// Simulate database query if enabled
	if dbSimulator != nil {
		// Simulate a typical read operation (e.g., fetching user data)
		result := dbSimulator.SimulateSelect(r.Context(), "users")
		if !result.Success {
			slog.Warn("simulated db query failed during ping",
				"method", r.Method,
				"path", r.URL.Path,
				"error_type", result.ErrorType,
			)
		}
	}

	n := rand.Float64() * 100
	if n <= successProb {
		slog.Debug("ping request succeeded", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		w.WriteHeader(200)
		_, _ = fmt.Fprintln(w, "pong")
	} else {
		slog.Warn("ping request failed", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "status", 500)
		w.WriteHeader(500)
	}
}

func runPongServer() (err error) {
	slog.Info("starting pong server", "build_info", version.Info(), "build_context", version.BuildContext())

	latDecider, err = newLatencyDecider(lat)
	if err != nil {
		return err
	}

	version.Version = appVersion

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		versioncollector.NewCollector("pong"),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Initialize database simulator if enabled
	if dbEnabled {
		dbMetrics := extdb.NewMetrics(reg, nil)
		dbSimulator, err = extdb.NewSimulator(dbMetrics, extdb.SimulatorOpts{
			Latency:     dbLatency,
			SuccessProb: dbSuccessProb,
			ErrorTypes:  dbErrorTypes,
		})
		if err != nil {
			return errors.Wrap(err, "creating database simulator")
		}
		slog.Info("database simulation enabled",
			"latency", dbLatency,
			"success_prob", dbSuccessProb,
			"error_types", dbErrorTypes,
		)
	}

	instr := exthttp.NewInstrumentationMiddleware(reg, []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120, 240, 360, 720})
	m := http.NewServeMux()
	m.Handle("/metrics", instr.NewHandler("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{},
	)))
	m.Handle("/ping", instr.NewHandler("/ping", http.HandlerFunc(handlerPing)))
	srv := http.Server{Addr: pongAddr, Handler: m}

	g := &run.Group{}
	g.Add(func() error {
		slog.Info("starting HTTP server", "address", pongAddr, "mode", "pong")
		if err := srv.ListenAndServe(); err != nil {
			return errors.Wrap(err, "starting web server")
		}
		return nil
	}, func(error) {
		slog.Info("shutting down HTTP server")
		if err := srv.Close(); err != nil {
			slog.Error("failed to stop web server", "error", err)
		}
	})
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))
	err = g.Run()
	var sigErr run.SignalError
	if errors.As(err, &sigErr) {
		slog.Info("received signal, shutting down")
		return nil
	}
	return err
}

func runPinger() (err error) {
	slog.Info("starting pinger", "build_info", version.Info(), "build_context", version.BuildContext())

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		versioncollector.NewCollector("ping"),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	instr := exthttp.NewInstrumentationMiddleware(reg, []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120, 240, 360, 720})
	m := http.NewServeMux()
	m.Handle("/metrics", instr.NewHandler("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{},
	)))
	srv := http.Server{Addr: pingAddr, Handler: m}

	g := &run.Group{}
	g.Add(func() error {
		slog.Info("starting HTTP server", "address", pingAddr, "mode", "ping")
		if err := srv.ListenAndServe(); err != nil {
			return errors.Wrap(err, "starting web server")
		}
		return nil
	}, func(error) {
		slog.Info("shutting down HTTP server")
		if err := srv.Close(); err != nil {
			slog.Error("failed to stop web server", "error", err)
		}
	})
	{
		client := &http.Client{
			Transport: exthttp.InstrumentedRoundTripper(http.DefaultTransport, exthttp.NewClientMetrics(reg)),
		}

		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			spamPings(ctx, client, endpoint, pingsPerSec)
			return nil
		}, func(error) {
			cancel()
		})
	}
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))
	err = g.Run()
	var sigErr run.SignalError
	if errors.As(err, &sigErr) {
		slog.Info("received signal, shutting down")
		return nil
	}
	return err
}

func spamPings(ctx context.Context, client *http.Client, endpoint string, pingsPerSec int) {
	slog.Info("starting ping spam", "endpoint", endpoint, "pings_per_sec", pingsPerSec)
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-time.After(1 * time.Second):
		}

		for range pingsPerSec {
			wg.Add(1)
			go ping(ctx, client, endpoint, &wg)
		}
	}
}

func ping(ctx context.Context, client *http.Client, endpoint string, wg *sync.WaitGroup) {
	defer wg.Done()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		slog.Error("failed to create request", "error", err, "endpoint", endpoint)
		return
	}
	res, err := client.Do(r)
	if err != nil {
		slog.Error("failed to send request", "error", err, "endpoint", endpoint)
		return
	}
	slog.Debug("ping sent successfully", "endpoint", endpoint, "status", res.StatusCode)
	if res.Body != nil {
		// We don't care about response, just release resources.
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}
}
