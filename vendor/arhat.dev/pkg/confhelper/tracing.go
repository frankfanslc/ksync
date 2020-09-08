// +build !nocloud,!notelemetry

package confhelper

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/pflag"
	otapiglobal "go.opentelemetry.io/otel/api/global"
	otapitrace "go.opentelemetry.io/otel/api/trace"
	otexporterotlp "go.opentelemetry.io/otel/exporters/otlp"
	otexporterjaeger "go.opentelemetry.io/otel/exporters/trace/jaeger"
	otexporterzipkin "go.opentelemetry.io/otel/exporters/trace/zipkin"
	otsdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"

	"arhat.dev/pkg/envhelper"
	"arhat.dev/pkg/log"
)

func FlagsForTracing(prefix string, c *TracingConfig) *pflag.FlagSet {
	fs := pflag.NewFlagSet("tracing", pflag.ExitOnError)

	fs.BoolVar(&c.Enabled, prefix+"enabled", false, "enable tracing")
	fs.StringVar(&c.Format, prefix+"format", "jaeger", "set tracing stats format")
	fs.StringVar(&c.EndpointType, prefix+"endpointType", "agent",
		"set endpoint type of collector (only used for jaeger)")
	fs.StringVar(&c.Endpoint, prefix+"endpoint", "", "set collector endpoint for tracing stats collection")
	fs.Float64Var(&c.SampleRate, prefix+"sampleRate", 1.0, "set tracing sample rate")
	fs.StringVar(&c.ReportedServiceName, prefix+"reportedServiceName",
		fmt.Sprintf("%s.%s", envhelper.ThisPodName(), envhelper.ThisPodNS()), "set service name used for tracing stats")
	fs.AddFlagSet(FlagsForTLSConfig(prefix+"tls", &c.TLS))

	return fs
}

type TracingConfig struct {
	// Enabled tracing stats
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Format of exposed tracing stats
	Format string `json:"format" yaml:"format"`

	// EndpointType the type of collector (used for jaeger), can be one of [agent, collector]
	EndpointType string `json:"endpointType" yaml:"endpointType"`

	// Endpoint to report tracing stats
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// SampleRate
	SampleRate float64 `json:"sampleRate" yaml:"sampleRate"`

	// ReportedServiceName used when reporting tracing stats
	ReportedServiceName string `json:"serviceName" yaml:"serviceName"`

	// TLS config for client/server
	TLS TLSConfig `json:"tls" yaml:"tls"`
}

func (c *TracingConfig) newHTTPClient(tlsConfig *tls.Config) *http.Client {
	if tlsConfig != nil {
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
	}

	// TODO: set reasonable defaults, currently using default client and transport
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:       30 * time.Second,
				KeepAlive:     30 * time.Second,
				FallbackDelay: 300 * time.Millisecond,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			DialTLS:                nil,
			TLSClientConfig:        tlsConfig,
			DisableKeepAlives:      false,
			DisableCompression:     false,
			MaxIdleConnsPerHost:    0,
			MaxConnsPerHost:        0,
			ResponseHeaderTimeout:  0,
			TLSNextProto:           nil,
			ProxyConnectHeader:     nil,
			MaxResponseHeaderBytes: 0,
			WriteBufferSize:        0,
			ReadBufferSize:         0,
		},
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       0,
	}
}

func (c *TracingConfig) RegisterIfEnabled(ctx context.Context, logger log.Interface) (err error) {
	if !c.Enabled {
		return nil
	}

	var traceProvider otapitrace.Provider

	tlsConfig, err := c.TLS.GetTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create tls config: %w", err)
	}

	switch c.Format {
	case "otlp":
		opts := []otexporterotlp.ExporterOption{
			otexporterotlp.WithAddress(c.Endpoint),
		}

		if tlsConfig != nil {
			opts = append(opts, otexporterotlp.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		} else {
			opts = append(opts, otexporterotlp.WithInsecure())
		}

		var exporter *otexporterotlp.Exporter
		exporter, err = otexporterotlp.NewExporter(opts...)
		if err != nil {
			return fmt.Errorf("failed to create otlp exporter: %w", err)
		}

		traceProvider, err = otsdktrace.NewProvider(
			otsdktrace.WithConfig(otsdktrace.Config{DefaultSampler: otsdktrace.ProbabilitySampler(c.SampleRate)}),
			otsdktrace.WithSyncer(exporter),
		)
		if err != nil {
			return fmt.Errorf("failed to create trace provider for otlp exporter: %w", err)
		}
	case "zipkin":
		var exporter *otexporterzipkin.Exporter

		exporter, err = otexporterzipkin.NewExporter(c.Endpoint, c.ReportedServiceName,
			otexporterzipkin.WithClient(c.newHTTPClient(tlsConfig)),
			otexporterzipkin.WithLogger(nil),
		)
		if err != nil {
			return fmt.Errorf("failed to create zipkin exporter: %w", err)
		}

		traceProvider, err = otsdktrace.NewProvider(
			otsdktrace.WithBatcher(exporter,
				otsdktrace.WithBatchTimeout(5*time.Second),
			),
			otsdktrace.WithConfig(otsdktrace.Config{
				DefaultSampler: otsdktrace.ProbabilitySampler(c.SampleRate),
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to create trace provider for zipkin exporter: %w", err)
		}
	case "jaeger":
		var endpoint otexporterjaeger.EndpointOption
		switch c.EndpointType {
		case "agent":
			endpoint = otexporterjaeger.WithAgentEndpoint(c.Endpoint)
		case "collector":
			otexporterjaeger.WithCollectorEndpoint(c.Endpoint,
				otexporterjaeger.WithUsername(os.Getenv("JAEGER_COLLECTOR_USERNAME")),
				otexporterjaeger.WithPassword(os.Getenv("JAEGER_COLLECTOR_PASSWORD")),
				otexporterjaeger.WithHTTPClient(c.newHTTPClient(tlsConfig)),
			)
		default:
			return fmt.Errorf("unsupported tracing endpoint type %q", c.EndpointType)
		}

		var flush func()
		traceProvider, flush, err = otexporterjaeger.NewExportPipeline(endpoint,
			otexporterjaeger.WithProcess(otexporterjaeger.Process{
				ServiceName: c.ReportedServiceName,
			}),
			otexporterjaeger.WithSDK(&otsdktrace.Config{
				DefaultSampler: otsdktrace.ProbabilitySampler(c.SampleRate),
			}),
		)
		_ = flush
	default:
		return fmt.Errorf("unsupported tracing format %q", c.Format)
	}

	if err != nil {
		return fmt.Errorf("failed to create %q tracing provider: %w", c.Format, err)
	}

	otapiglobal.SetTraceProvider(traceProvider)

	return nil
}
