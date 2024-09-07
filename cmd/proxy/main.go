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

	configDir := filepath.Dir(viper.ConfigFileUsed())
	viper.SetDefault("bindAddress", ":8080")
	viper.SetDefault("serviceUpdateInterval", time.Second*30)
	viper.SetDefault("cors.allowedHeaders", []string{"*"})
	viper.SetDefault("tls.enabled", true)
	viper.SetDefault("tls.certFile", filepath.Join(configDir, "server.crt"))
	viper.SetDefault("tls.keyFile", filepath.Join(configDir, "server.key"))
	viper.SetDefault("pprof.enabled", true)
	viper.SetDefault("docker.enabled", true)

	var logger *zap.Logger
	if viper.GetBool("logger.development") {
		logger = zap.Must(zap.NewDevelopment())
	} else {
		logger = zap.Must(zap.NewProduction())
	}

	zap.ReplaceGlobals(logger.Named("global"))

	ctx := context.Background()
	staticProvider := providers.Static{
		ServiceUpdateInterval: viper.GetDuration("serviceUpdateInterval"),
		Servers:               getStaticServers(logger),
	}

	dockerProvider := providers.Docker{
		ExposeMode:      providers.ExposeMode(viper.GetString("docker.expose")),
		Label:           viper.GetString("docker.label"),
		DockerHost:      viper.GetString("docker.host"),
		ExposedProjects: viper.GetStringSlice("docker.exposedProjects"),
		DefaultNetwork:  viper.GetString("docker.network"),
		Logger:          logger.Named("docker_provider"),
	}

	srv := proxy.NewServer(proxy.ProxyConfig{
		DisableReflection: viper.GetBool("disableReflection"),
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

	var handler http.Handler
	if viper.GetBool("cors.enabled") {
		cors := cors.New(cors.Options{
			AllowedOrigins: viper.GetStringSlice("cors.allowedOrigins"),
			AllowedMethods: []string{"POST", "OPTIONS"},
			AllowedHeaders: viper.GetStringSlice("cors.allowedHeaders"),
			ExposedHeaders: []string{"Grpc-Status", "Grpc-Message"},
		})
		handler = cors.Handler(srv)
	} else {
		handler = srv
	}

	addr := viper.GetString("bindAddress")
	logger.Info("Starting proxy", zap.String("address", addr))

	var err error
	if viper.GetBool("tls.enabled") {
		err = http.ListenAndServeTLS(addr, viper.GetString("tls.certFile"), viper.GetString("tls.keyFile"), handler)
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
		Addr:    "localhost:6060",
	}

	logger.Info("Starting pprof server", zap.String("address", srv.Addr))
	err := srv.ListenAndServe()
	logger.Error("pprof server stopped", zap.Error(err))
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
