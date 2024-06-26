package main

import (
	"context"
	"log"
	"main/proxy"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func main() {
	viper.SetDefault("bind_address", ":8080")
	viper.SetDefault("service_update_interval", time.Second*30)

	viper.AddConfigPath("/etc/pancake")
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("pancake")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	var upstreams []proxy.UpstreamConfig
	if err := viper.UnmarshalKey("servers", &upstreams); err != nil {
		log.Fatalln("Config: servers key is bad", err)
	}

	srv := proxy.NewServer(proxy.ServerConfig{
		Upstreams:             upstreams,
		ServiceUpdateInterval: viper.GetDuration("service_update_interval"),
	})

	go func() {
		err := srv.RunBackgroundLoop(context.Background())
		panic(err)
	}()

	addr := viper.GetString("bind_address")
	log.Printf("Starting proxy on %s\n", addr)
	err := http.ListenAndServeTLS(addr, "server.crt", "server.key", srv)
	log.Fatalln(err)
}
