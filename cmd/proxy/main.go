package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/natk64/pancake-proxy/proxy"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func main() {
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

	var logger *zap.Logger
	if viper.GetBool("logger.development") {
		logger = zap.Must(zap.NewDevelopment())
	} else {
		logger = zap.Must(zap.NewProduction())
	}

	zap.ReplaceGlobals(logger.Named("global"))

	var config proxy.ServerConfig
	if err := viper.Unmarshal(&config); err != nil {
		logger.Fatal("Failed to unmarshal config", zap.Error(err))
	}

	config.Logger = logger.Named("server")
	srv := proxy.NewServer(config)

	go func() {
		err := srv.RunProxy(context.Background())
		panic(err)
	}()

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
