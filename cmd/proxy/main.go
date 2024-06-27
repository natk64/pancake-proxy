package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/NicoKleinschmidt/pancake-proxy/proxy"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func main() {
	viper.SetDefault("bindAddress", ":8080")
	viper.SetDefault("serviceUpdateInterval", time.Second*30)

	viper.AddConfigPath("/etc/pancake")
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("pancake")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	var logger *zap.Logger
	if viper.GetBool("logger.development") {
		logger = zap.Must(zap.NewDevelopment())
	} else {
		logger = zap.Must(zap.NewProduction())
	}

	zap.ReplaceGlobals(logger.Named("global"))

	var config proxy.ServerConfig
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalln("Failed to unmarshal config", err)
	}

	config.Logger = logger.Named("server")
	srv := proxy.NewServer(config)

	go func() {
		err := srv.RunProxy(context.Background())
		panic(err)
	}()

	addr := viper.GetString("bindAddress")
	log.Printf("Starting proxy on %s\n", addr)
	err := http.ListenAndServeTLS(addr, "server.crt", "server.key", srv)
	log.Fatalln(err)
}
