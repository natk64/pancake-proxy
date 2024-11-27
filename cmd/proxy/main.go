package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/natk64/pancake-proxy/providers"
	"github.com/natk64/pancake-proxy/proxy"
	"github.com/natk64/pancake-proxy/utils"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"net/http/pprof"
)

func main() {
	// Reset default serve mux to remove default pprof routes
	http.DefaultServeMux = http.NewServeMux()

	viper.AddConfigPath("/etc/pancake")
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("pancake")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	aliases := map[string]string{
		"serviceUpdateInterval":  "service_update_interval",
		"disableReflection":      "disable_reflection",
		"bindAddress":            "bind_address",
		"pprof.bindAddress":      "pprof.bind_address",
		"dashboard.bindAddress":  "dashboard.bind_address",
		"tls.certFile":           "tls.cert_file",
		"tls.keyFile":            "tls.key_file",
		"docker.exposedProjects": "docker.exposed_projects",
		"cors.allowedOrigins":    "cors.allowed_origins",
		"cors.allowedHeaders":    "cors.allowed_headers",
	}

	for old, new := range aliases {
		if !viper.InConfig(new) {
			viper.Set(new, viper.Get(old))
		}
	}

	configDir := filepath.Dir(viper.ConfigFileUsed())
	viper.SetDefault("bind_address", ":8080")
	viper.SetDefault("service_update_interval", time.Second*30)
	viper.SetDefault("cors.allowed_headers", []string{"*"})
	viper.SetDefault("tls.enabled", true)
	viper.SetDefault("tls.cert_file", filepath.Join(configDir, "server.crt"))
	viper.SetDefault("tls.key_file", filepath.Join(configDir, "server.key"))
	viper.SetDefault("pprof.enabled", false)
	viper.SetDefault("pprof.bind_address", "localhost:6060")
	viper.SetDefault("docker.enabled", false)
	viper.SetDefault("dashboard.enabled", false)
	viper.SetDefault("dashboard.bind_address", ":8081")

	var logger *zap.Logger
	if viper.GetBool("logger.development") {
		logger = zap.Must(zap.NewDevelopment())
	} else {
		logger = zap.Must(zap.NewProduction())
	}

	zap.ReplaceGlobals(logger.Named("global"))

	ctx := context.Background()
	staticProvider := providers.Static{
		ServiceUpdateInterval: viper.GetDuration("service_update_interval"),
		Servers:               getStaticServers(logger),
	}

	dockerProvider := providers.Docker{
		ExposeMode:      providers.ExposeMode(viper.GetString("docker.expose")),
		Label:           viper.GetString("docker.label"),
		DockerHost:      viper.GetString("docker.host"),
		ExposedProjects: viper.GetStringSlice("docker.exposed_projects"),
		DefaultNetwork:  viper.GetString("docker.network"),
		Logger:          logger.Named("docker_provider"),
	}

	srv := proxy.NewServer(proxy.ProxyConfig{
		DisableReflection: viper.GetBool("disable_reflection"),
		Logger:            logger.Named("server"),
	})

	go utils.AutoRestarter{
		Name:   "Static provider",
		Delay:  time.Second * 10,
		Logger: logger,
		F: func(ctx context.Context) error {
			return staticProvider.Run(ctx, srv)
		},
	}.Run(ctx)

	if viper.GetBool("docker.enabled") {
		go utils.AutoRestarter{
			Name:   "Docker provider",
			Delay:  time.Second * 10,
			Logger: logger,
			F: func(ctx context.Context) error {
				return dockerProvider.Run(ctx, srv)
			},
		}.Run(ctx)
	}

	if viper.GetBool("pprof.enabled") {
		go runPprofListener(logger.Named("pprof_server"))
	}

	if viper.GetBool("dashboard.enabled") {
		go runDashboardListener(srv, logger, viper.GetString("dashboard.bind_address"))
	}

	var handler http.Handler
	if viper.GetBool("cors.enabled") {
		cors := cors.New(cors.Options{
			AllowedOrigins: viper.GetStringSlice("cors.allowed_origins"),
			AllowedMethods: []string{"POST", "OPTIONS"},
			AllowedHeaders: viper.GetStringSlice("cors.allowed_headers"),
			ExposedHeaders: []string{"Grpc-Status", "Grpc-Message"},
		})
		handler = cors.Handler(srv)
	} else {
		handler = srv
	}

	addr := viper.GetString("bind_address")
	logger.Info("Starting proxy", zap.String("address", addr))

	var err error
	if viper.GetBool("tls.enabled") {
		err = http.ListenAndServeTLS(addr, viper.GetString("tls.cert_file"), viper.GetString("tls.key_file"), handler)
	} else {
		err = http.ListenAndServe(addr, handler)
	}
	logger.Fatal("Server stopped", zap.Error(err))
}

func runPprofListener(logger *zap.Logger) {
	pprofServeMux := http.NewServeMux()
	pprofServeMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofServeMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofServeMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofServeMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofServeMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Handler: pprofServeMux,
		Addr:    viper.GetString("pprof.bind_address"),
	}

	logger.Info("Starting pprof server", zap.String("address", srv.Addr))
	err := srv.ListenAndServe()
	logger.Error("pprof server stopped", zap.Error(err))
}

func runDashboardListener(p *proxy.Proxy, logger *zap.Logger, addr string) {
	dashboardServeMux := http.NewServeMux()
	dashboardServeMux.HandleFunc("/{$}", p.DashboardHandler)

	srv := &http.Server{
		Handler: dashboardServeMux,
		Addr:    addr,
	}

	logger.Info("Starting dashboard server", zap.String("address", srv.Addr))
	err := srv.ListenAndServe()
	logger.Error("Dashboard server stopped", zap.Error(err))
}

func getStaticServers(logger *zap.Logger) []proxy.UpstreamConfig {
	type config struct {
		Servers []proxy.UpstreamConfig `mapstructure:"servers"`
	}

	var conf config
	if err := viper.Unmarshal(&conf); err != nil {
		logger.Fatal("Failed to load static server config", zap.Error(err))
	}
	return conf.Servers
}
