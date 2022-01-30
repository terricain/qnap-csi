package main

//package main
//
//import (
//	"context"
//	"flag"
//	"fmt"
//	"log"
//	"os"
//	"os/signal"
//	"syscall"
//
//	"github.com/digitalocean/csi-digitalocean/driver"
//)
//

import (
	"context"
	"flag"
	"fmt"
	iscsiLib "github.com/kubernetes-csi/csi-lib-iscsi/iscsi"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/terrycain/qnap-csi/driver"
	"os"
	"os/signal"
	"syscall"
)


func main() {
	var (
		endpoint   = flag.String("endpoint", "unix:///var/run/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		qnapURL    = flag.String("url", "", "QNAP URL")
		logLevel   = flag.String("log-level", "info", "Log level (info/warn/fatal/error)")
		version    = flag.Bool("version", false, "Print the version and exit")
		controller = flag.Bool("controller", false, "Serve controller driver, else it will operate as node driver")
		prefix 	   = flag.String("prefix", driver.DefaultVolumePrefix, "Naming prefix")
		nodeID 	   = flag.String("node-id", "", "Node ID")
		portal 	   = flag.String("portal", "", "Portal Address (IP:PORT)")
		storagePoolID = flag.Int("storage-pool-id", 1, "Storage Pool ID")
	)
	flag.Parse()

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.Version, driver.Commit, driver.GitTreeState)
		os.Exit(0)
	}

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse log level")
	}
	zerolog.SetGlobalLevel(level)

	var drv *driver.Driver

	if *nodeID == "" {
		log.Fatal().Msg("Node ID must be specified")
	}

	if *controller {
		username := os.Getenv("QNAP_USERNAME")
		password := os.Getenv("QNAP_PASSWORD")
		log.Debug().Msg("Initiating controller driver")
		if drv, err = driver.NewDriver(*endpoint, *qnapURL, username, password, *controller, *prefix, *nodeID, *portal, *storagePoolID); err != nil {
			log.Fatal().Err(err).Msg("Failed to init CSI driver")
		}
	} else {
		if *logLevel == "debug" {
			iscsiLib.EnableDebugLogging(os.Stdout)
		}

		// Node mode doesnt require qnap access
		log.Debug().Msg("Initiating node driver")
		if drv, err = driver.NewDriver(*endpoint, *qnapURL, "", "", *controller, *prefix, *nodeID, *portal, *storagePoolID); err != nil {
			log.Fatal().Err(err).Msg("Failed to init CSI driver")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Info().Msgf("Caught signal %s", sig.String())
		cancel()
	}()

	if err = drv.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to run CSI driver")
	}
}
