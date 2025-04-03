package main

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"text/template"
	"time"

	arg "github.com/alexflint/go-arg"
	"github.com/digitalocean/godo"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/joho/godotenv"
	"github.com/metalmatze/digitalocean_exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
)

const (
	rootTemplate = `
{{- define "content" }}
<!DOCTYPE html>
<html lang="en-US">
<head>
	<meta charset="utf-8">
	<title>DigitalOcean Exporter</title>
	<style>
	body {
  		font-family: Verdana;
	}
	</style>
</head>
<body>
	<h2>DigitalOcean Exporter</h2>
	<li><a href="{{ .MetricsPath }}">metrics</a></li>
	<li><a href="/healthz">healthz</a></li>
</body>
</html>
{{- end }}`
)

var (
	// Version of digitalocean_exporter.
	Version string
	// Revision or Commit this binary was built from.
	Revision string
	// BuildDate this binary was built.
	BuildDate string
	// GoVersion running this binary.
	GoVersion = runtime.Version()
	// StartTime has the time this was started.
	StartTime = time.Now()
)

// Config gets its content from env and passes it on to different packages
type Config struct {
	Debug                 bool   `arg:"env:DEBUG"`
	DigitalOceanToken     string `arg:"env:DIGITALOCEAN_TOKEN"`
	SpacesAccessKeyID     string `arg:"env:DIGITALOCEAN_SPACES_ACCESS_KEY_ID"`
	SpacesAccessKeySecret string `arg:"env:DIGITALOCEAN_SPACES_ACCESS_KEY_SECRET"`
	HTTPTimeout           int    `arg:"env:HTTP_TIMEOUT"`
	WebAddr               string `arg:"env:WEB_ADDR"`
	WebPath               string `arg:"env:WEB_PATH"`
}

// Token returns a token or an error.
func (c Config) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: c.DigitalOceanToken}, nil
}

// Content is used by the root handler's tempalte
type Content struct {
	MetricsPath string
}

func main() {
	_ = godotenv.Load()

	c := Config{
		HTTPTimeout: 5000,
		WebPath:     "/metrics",
		WebAddr:     ":9212",
	}
	arg.MustParse(&c)

	if c.DigitalOceanToken == "" {
		panic("DigitalOcean Token is required")
	}

	filterOption := level.AllowInfo()
	if c.Debug {
		filterOption = level.AllowDebug()
	}

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = level.NewFilter(logger, filterOption)
	logger = log.With(logger,
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)

	// nolint:errcheck
	level.Info(logger).Log(
		"msg", "starting digitalocean_exporter",
		"version", Version,
		"revision", Revision,
		"buildDate", BuildDate,
		"goVersion", GoVersion,
	)

	if c.SpacesAccessKeyID == "" && c.SpacesAccessKeySecret == "" {
		// nolint:errcheck
		level.Warn(logger).Log(
			"msg", "Spaces Access Key ID and Secret unset. Spaces buckets will not be collected",
		)
	}

	oauthClient := oauth2.NewClient(context.TODO(), c)

	// Automatic Retries and Exponential Backoff
	// https://github.com/digitalocean/godo?tab=readme-ov-file#automatic-retries-and-exponential-backoff
	waitMax := godo.PtrTo(6.0)
	waitMin := godo.PtrTo(3.0)

	retryConfig := godo.RetryConfig{
		RetryMax:     3,
		RetryWaitMin: waitMin,
		RetryWaitMax: waitMax,
	}

	client, err := godo.New(oauthClient, godo.WithRetryAndBackoffs(retryConfig))
	if err != nil {
		level.Error(logger).Log(
			"msg", "unable to create DigitalOcean API instance",
			"err", err,
		)
		os.Exit(1)
	}

	timeout := time.Duration(c.HTTPTimeout) * time.Millisecond

	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "digitalocean_errors_total",
		Help: "The total number of errors per collector",
	}, []string{"collector"})

	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	r.MustRegister(collectors.NewGoCollector())
	r.MustRegister(errors)
	r.MustRegister(collector.NewExporterCollector(logger, Version, Revision, BuildDate, GoVersion, StartTime))
	r.MustRegister(collector.NewAccountCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewAppCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewBalanceCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewDBCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewDomainCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewDropletCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewFloatingIPCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewImageCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewKeyCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewLoadBalancerCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewSnapshotCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewVolumeCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewKubernetesCollector(logger, errors, client, timeout))
	r.MustRegister(collector.NewIncidentCollector(logger, errors, timeout))

	// Only run spaces bucket collector if access key id and secret are set
	if c.SpacesAccessKeyID != "" && c.SpacesAccessKeySecret != "" {
		r.MustRegister(collector.NewSpacesCollector(logger, errors, client, c.SpacesAccessKeyID, c.SpacesAccessKeySecret, timeout))
	}

	http.Handle(c.WebPath,
		promhttp.HandlerFor(r, promhttp.HandlerOpts{}),
	)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		t := template.Must(template.New("content").Parse(rootTemplate))
		if err := t.ExecuteTemplate(w, "content", Content{MetricsPath: c.WebPath}); err != nil {
			// nolint:errcheck
			level.Error(logger).Log("msg", "unable to execute template", err)
		}
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			// nolint:errcheck
			level.Error(logger).Log("msg", "unable to write response", err)
		}
	})

	// nolint:errcheck
	level.Info(logger).Log("msg", "listening", "addr", c.WebAddr)
	if err := http.ListenAndServe(c.WebAddr, nil); err != nil {
		// nolint:errcheck
		level.Error(logger).Log("msg", "http listenandserve error", "err", err)
		os.Exit(1)
	}
}
