// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	. "github.com/rgeyer/ssm2logger/ssm2lib"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logfile_path string
var defsPath string
var logFormat string
var paramsCsv string
var allParams bool
var maxAddresses int
var unixSocketPath string

type ndjsonSample struct {
	Ts    int64              `json:"ts"`
	RomID string             `json:"rom_id"`
	SsmID string             `json:"ssm_id"`
	Data  map[string]float64 `json:"data"`
}

// logCmd represents the log command
var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Logs SSM2 data and writes CSV or NDJSON samples",
	RunE: func(cmd *cobra.Command, args []string) error {
		if logFormat != "csv" && logFormat != "ndjson" {
			return fmt.Errorf("unsupported format %q; expected csv or ndjson", logFormat)
		}
		if unixSocketPath != "" && logFormat != "ndjson" {
			return fmt.Errorf("--unix-socket can only be used with --format ndjson")
		}

		logDefs, err := loadLoggerDefinitions(defsPath)
		if err != nil {
			return err
		}

		ssm2_conn := &Ssm2Connection{}
		ssm2_conn.SetLogger(logger)

		if err := ssm2_conn.Open(port); err != nil {
			return err
		}
		defer ssm2_conn.Close()

		initResponse, err := ssm2_conn.InitEngine()
		if err != nil {
			return err
		}

		allSsmParams := getSsmProtocolParameters(logDefs)
		supportedParams := getSupportedParameters(allSsmParams, initResponse.GetCapabilityBytes())

		logger.WithFields(log.Fields{
			"SsmId":                  hex.EncodeToString(initResponse.GetSsmId()),
			"RomId":                  hex.EncodeToString(initResponse.GetRomId()),
			"Supported Capabilities": len(supportedParams),
		}).Info("Initialized ECM")

		selection, err := selectParameters(supportedParams, allParams, paramsCsv, maxAddresses)
		if err != nil {
			return err
		}
		if selection.Trimmed {
			logger.WithFields(log.Fields{"max_addresses": maxAddresses, "selected_params": len(selection.Params), "wanted_addresses": selection.Wanted}).Warn("Requested parameters exceed max address count and were trimmed")
		}
		if len(selection.Params) == 0 {
			return fmt.Errorf("no parameters selected; check --params/--all and ECU capability support")
		}

		addresses, mappings, err := BuildParameterAddressRequest(selection.Params)
		if err != nil {
			return err
		}
		if len(addresses) > maxAddresses {
			return fmt.Errorf("selected params would request %d addresses, which exceeds max of %d", len(addresses), maxAddresses)
		}

		compiledMappings, err := buildCompiledMappings(mappings)
		if err != nil {
			return err
		}

		// Cooldown between writes
		time.Sleep(200 * time.Millisecond)

		sigs := make(chan os.Signal, 1)
		loop := true
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			for sig := range sigs {
				if sig == syscall.SIGINT || sig == syscall.SIGTERM {
					loop = false
				}
			}
		}()

		if _, err := ssm2_conn.ReadAddressesContinous(addresses); err != nil {
			return err
		}

		if logFormat == "ndjson" {
			return streamNdjson(ssm2_conn, &loop, initResponse, compiledMappings, len(addresses), unixSocketPath)
		}
		return streamCsv(ssm2_conn, &loop, initResponse, compiledMappings, len(addresses))
	},
}

func streamCsv(ssm2Conn *Ssm2Connection, loop *bool, initResponse *Ssm2InitResponsePacket, mappings []compiledMapping, requestedAddressCount int) error {
	timestamp := time.Now()
	logfilename := fmt.Sprintf("%s/%s-%d-log.csv", logfile_path, hex.EncodeToString(initResponse.GetRomId()), timestamp.Unix())

	csvfile, err := os.Create(logfilename)
	if err != nil {
		return err
	}
	defer csvfile.Close()

	writer := csv.NewWriter(csvfile)
	defer writer.Flush()

	header := []string{"timestamp"}
	for _, mapping := range mappings {
		header = append(header, formatHeaderLabel(mapping.ParameterMapping))
	}
	writer.Write(header)

	for *loop {
		readPacket, err := ssm2Conn.GetNextPacketInStream()
		if err != nil {
			return err
		}
		payload := readPacket.GetPayloadBytes()
		if len(payload) != requestedAddressCount {
			logger.WithFields(log.Fields{"expected_payload": requestedAddressCount, "actual_payload": len(payload)}).Debug("Skipping sample due to unexpected payload length")
			continue
		}

		row := []string{strconv.FormatInt(time.Now().Unix(), 10)}
		for _, mapping := range mappings {
			convertedValue, err := evaluateCompiledMapping(mapping, payload)
			if err != nil {
				return err
			}
			row = append(row, strconv.FormatFloat(convertedValue, 'f', 6, 64))
		}

		writer.Write(row)
	}

	logger.Info("Received Stop Signal and discontinued logging")
	return nil
}

func streamNdjson(ssm2Conn *Ssm2Connection, loop *bool, initResponse *Ssm2InitResponsePacket, mappings []compiledMapping, requestedAddressCount int, socketPath string) error {
	writer, closeFn, err := ndjsonWriter(socketPath)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	encoder := json.NewEncoder(writer)
	romID := hex.EncodeToString(initResponse.GetRomId())
	ssmID := hex.EncodeToString(initResponse.GetSsmId())

	for *loop {
		readPacket, err := ssm2Conn.GetNextPacketInStream()
		if err != nil {
			return err
		}
		payload := readPacket.GetPayloadBytes()
		if len(payload) != requestedAddressCount {
			logger.WithFields(log.Fields{"expected_payload": requestedAddressCount, "actual_payload": len(payload)}).Debug("Skipping sample due to unexpected payload length")
			continue
		}

		data := make(map[string]float64, len(mappings))
		for _, mapping := range mappings {
			convertedValue, err := evaluateCompiledMapping(mapping, payload)
			if err != nil {
				return err
			}
			data[mapping.ndjsonKey] = convertedValue
		}

		sample := ndjsonSample{
			Ts:    time.Now().UnixMilli(),
			RomID: romID,
			SsmID: ssmID,
			Data:  data,
		}
		if err := encoder.Encode(sample); err != nil {
			return err
		}
	}

	logger.Info("Received Stop Signal and discontinued logging")
	return nil
}

func init() {
	rootCmd.AddCommand(logCmd)

	logCmd.Flags().StringVar(&logfile_path, "logfile-path", ".", "Path where the logfile will be generated. The actual file will be <logfile-path>/<ecu romid>-<timestamp>-log.csv.")
	logCmd.Flags().StringVar(&defsPath, "defs", "logger_STD_EN_v336.xml", "Path to RomRaider logger definition XML")
	logCmd.Flags().StringVar(&logFormat, "format", "csv", "Output format: csv or ndjson")
	logCmd.Flags().StringVar(&paramsCsv, "params", "", "Comma-separated list of parameter names to log")
	logCmd.Flags().BoolVar(&allParams, "all", false, "Log all supported parameters (subject to --max-addresses)")
	logCmd.Flags().IntVar(&maxAddresses, "max-addresses", 45, "Maximum number of ECU addresses to request in a single logging packet")
	logCmd.Flags().StringVar(&unixSocketPath, "unix-socket", "", "Unix domain socket path for NDJSON output (requires --format ndjson)")

	viper.BindPFlag("logfile-path", logCmd.Flags().Lookup("logfile-path"))
	viper.SetDefault("logfile-path", ".")
}

func ndjsonWriter(socketPath string) (io.Writer, func() error, error) {
	if socketPath == "" {
		return os.Stdout, nil, nil
	}

	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return nil, nil, fmt.Errorf("failed to remove existing unix socket %q: %w", socketPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("failed to inspect unix socket path %q: %w", socketPath, err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen on unix socket %q: %w", socketPath, err)
	}

	logger.WithField("unix_socket", socketPath).Info("Waiting for unix socket client connection")
	conn, err := listener.Accept()
	if err != nil {
		listener.Close()
		os.Remove(socketPath)
		return nil, nil, fmt.Errorf("failed to accept unix socket client on %q: %w", socketPath, err)
	}

	closeFn := func() error {
		connErr := conn.Close()
		listenerErr := listener.Close()
		removeErr := os.Remove(socketPath)
		if connErr != nil {
			return connErr
		}
		if listenerErr != nil {
			return listenerErr
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}

	return conn, closeFn, nil
}
