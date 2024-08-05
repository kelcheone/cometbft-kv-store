package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	abciserver "github.com/cometbft/cometbft/abci/server"
	cfg "github.com/cometbft/cometbft/config"
	cmtflags "github.com/cometbft/cometbft/libs/cli/flags"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	nm "github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/dgraph-io/badger/v4"
	"github.com/spf13/viper"
)

var (
	homedir    string
	socketAddr string
)

func init() {
	flag.StringVar(&homedir, "cmt-home", "", "Path to the CometBFT config directory (if empty, uses $HOME/.cometbft/")
	flag.StringVar(&socketAddr, "socket-addr", "unix://example.sock", "Unix domain socket address (if empty uses \"unix://example.sock\")")
}

func sameProcess() {
	flag.Parse()
	if homedir == "" {
		homedir = os.ExpandEnv("$HOME/.cometbft/")
	}
	config := cfg.DefaultConfig()
	config.SetRoot(homedir)

	configTomlPath := filepath.Join(homedir, "config/config.toml")
	log.Println(configTomlPath)
	// viper.SetConfigFile(fmt.Sprintf("%s%s", homedir, "config/config.toml"))
	viper.SetConfigFile(configTomlPath)

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Reading config: %v", err)
	}

	if err := viper.Unmarshal(config); err != nil {
		log.Fatalf("Decoding config: %v", err)
	}

	if err := config.ValidateBasic(); err != nil {
		log.Fatalf("Invalid configuration data: %v", err)
	}
	dbPath := filepath.Join(homedir, "badger")
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		log.Fatalf("Opening database: %v", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("Closing databae: %v", err)
		}
	}()

	app := NewKVStoreApplication(db)

	pv := privval.LoadFilePV(
		config.PrivValidatorKeyFile(),
		config.PrivValidatorStateFile(),
	)

	nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
	if err != nil {
		log.Fatalf("failed to load node's key: %v", err)
	}

	logger := cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
	logger, err = cmtflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel)
	if err != nil {
		log.Fatalf("failed to parse log level: %v", err)
	}

	node, err := nm.NewNode(
		config,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(config),
		cfg.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		logger,
	)
	if err != nil {
		log.Fatalf("Creating node: %v", err)
	}

	node.Start()

	defer func() {
		node.Stop()
		node.Wait()
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}

func diffProcesses() {
	flag.Parse()
	if homedir == "" {
		homedir = os.ExpandEnv("$HOME/.cometbft/")
	}
	dbPath := filepath.Join(homedir, "badger")
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		log.Fatalf("Opening database: %v", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("Closing databae: %v", err)
		}
	}()

	app := NewKVStoreApplication(db)
	logger := cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
	server := abciserver.NewSocketServer(socketAddr, app)
	server.SetLogger(logger)

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting socket server: %v", err)
		os.Exit(1)
	}

	defer server.Stop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	<-c
}

func main() {
	// sameProcess()
	diffProcesses()
}
