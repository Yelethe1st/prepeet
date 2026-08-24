package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// Traces answer "what happened to this request". Metrics answer "is this
// getting worse", which is the question the scaling triggers in ADR-0006 are
// written against and the one a trace cannot answer cheaply.

// metered installs an in-memory meter provider and returns the handler plus a
// function that collects what was recorded.
func metered(t *testing.T, routes func(*http.ServeMux)) (http.Handler, func() metricdata.ResourceMetrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetMeterProvider(previous)
	})

	collect := func() metricdata.ResourceMetrics {
		var collected metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &collected); err != nil {
			t.Fatalf("collecting metrics: %v", err)
		}
		return collected
	}

	return httpserver.New(httpserver.Config{Routes: routes}), collect
}

// findMetric returns the named metric, or fails saying what was recorded
// instead, which is the information needed to fix the test or the code.
func findMetric(t *testing.T, collected metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()

	var seen []string
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric
			}
			seen = append(seen, metric.Name)
		}
	}
	t.Fatalf("no metric named %q was recorded; recorded: %v", name, seen)
	return metricdata.Metrics{}
}

// Request duration is the measurement three of the four ADR-0006 triggers
// depend on. Without it every threshold in that table stays an estimate.
func TestRequestDurationIsRecorded(t *testing.T) {
	handler, collect := metered(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))

	metric := findMetric(t, collect(), "http.server.request.duration")

	histogram, ok := metric.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("request duration is %T, want a float histogram: an average hides the tail that matters", metric.Data)
	}
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("recorded %d data points for one request, want 1", len(histogram.DataPoints))
	}
	if histogram.DataPoints[0].Count != 1 {
		t.Errorf("count = %d, want 1", histogram.DataPoints[0].Count)
	}
	if metric.Unit != "s" {
		t.Errorf("unit = %q, want seconds, which is what the semantic convention specifies", metric.Unit)
	}
}

// Attributes decide whether the measurement can answer anything. Route and
// status are what turn "the p99 is bad" into "the p99 on this endpoint is bad".
func TestRequestDurationIsBrokenDownByRouteAndStatus(t *testing.T) {
	handler, collect := metered(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/sessions/01a0301d-aa10-7000-8f3e-1234567890ab", nil))

	point := findMetric(t, collect(), "http.server.request.duration").
		Data.(metricdata.Histogram[float64]).DataPoints[0]

	want := map[attribute.Key]string{
		"http.route":                "GET /api/v1/sessions/{sessionID}",
		"http.request.method":       "GET",
		"http.response.status_code": "404",
	}
	for key, expected := range want {
		got, present := point.Attributes.Value(key)
		if !present {
			t.Errorf("%s is missing, so the measurement cannot be attributed", key)
			continue
		}
		if got.Emit() != expected {
			t.Errorf("%s = %q, want %q", key, got.Emit(), expected)
		}
	}
}

// A metric attribute is repeated on every series, so an unbounded one is worse
// here than on a span: it multiplies storage rather than adding a row. The
// request identifier is unique per request and must never become a dimension.
func TestRequestDurationCarriesNoUnboundedDimension(t *testing.T) {
	handler, collect := metered(t, nil)

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/sessions/01a0301d-aa10-7000-8f3e-1234567890ab", nil))

	point := findMetric(t, collect(), "http.server.request.duration").
		Data.(metricdata.Histogram[float64]).DataPoints[0]

	for _, attr := range point.Attributes.ToSlice() {
		if attr.Key == attribute.Key(telemetry.KeyRequestID) {
			t.Error("the request identifier is a metric dimension, which creates one time series per request")
		}
		if attr.Value.Emit() == "01a0301d-aa10-7000-8f3e-1234567890ab" {
			t.Errorf("%s carries an identifier from the path, which is an unbounded dimension", attr.Key)
		}
	}
}

// Two requests to the same route share a series, or aggregation is impossible.
func TestRequestsToTheSameRouteAggregate(t *testing.T) {
	handler, collect := metered(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {})
	})

	for _, path := range []string{"/api/v1/sessions/aaa", "/api/v1/sessions/bbb"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	points := findMetric(t, collect(), "http.server.request.duration").
		Data.(metricdata.Histogram[float64]).DataPoints
	if len(points) != 1 {
		t.Fatalf("two requests to one route produced %d series, want 1", len(points))
	}
	if points[0].Count != 2 {
		t.Errorf("count = %d, want 2", points[0].Count)
	}
}

// A panic must still be measured. Otherwise the requests that failed worst are
// the ones missing from the latency distribution.
func TestAPanickingRequestIsStillMeasured(t *testing.T) {
	handler, collect := metered(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
			panic("nil pool")
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	point := findMetric(t, collect(), "http.server.request.duration").
		Data.(metricdata.Histogram[float64]).DataPoints[0]

	if point.Count != 1 {
		t.Errorf("count = %d, want 1: a panicking request was not measured", point.Count)
	}
	if status, _ := point.Attributes.Value("http.response.status_code"); status.Emit() != "500" {
		t.Errorf("status = %q, want 500", status.Emit())
	}
}
