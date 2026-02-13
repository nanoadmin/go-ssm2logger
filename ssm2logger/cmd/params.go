package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	. "github.com/rgeyer/ssm2logger/ssm2lib"
	"github.com/spf13/cobra"
)

var paramsDefsPath string
var paramsFormat string

type paramOutput struct {
	Name         string `json:"name"`
	Units        string `json:"units"`
	Address      string `json:"address"`
	Length       int    `json:"length"`
	EcuByteIndex uint   `json:"ecu_byte_index"`
	EcuBit       uint   `json:"ecu_bit"`
	Supported    bool   `json:"supported"`
}

var paramsCmd = &cobra.Command{
	Use:   "params",
	Short: "List ECU parameters from logger definitions and ECU capabilities",
	RunE: func(cmd *cobra.Command, args []string) error {
		if paramsFormat != "text" && paramsFormat != "ndjson" {
			return fmt.Errorf("unsupported format %q; expected text or ndjson", paramsFormat)
		}

		logDefs, err := loadLoggerDefinitions(paramsDefsPath)
		if err != nil {
			return err
		}

		ssm2Conn := &Ssm2Connection{}
		ssm2Conn.SetLogger(logger)
		if err := ssm2Conn.Open(port); err != nil {
			return err
		}
		defer ssm2Conn.Close()

		initResponse, err := ssm2Conn.InitEngine()
		if err != nil {
			return err
		}

		allParams := getSsmProtocolParameters(logDefs)
		supported := getSupportedParameters(allParams, initResponse.GetCapabilityBytes())
		supportedMap := map[string]bool{}
		for _, p := range supported {
			supportedMap[p.Id] = true
		}

		if paramsFormat == "text" {
			fmt.Printf("rom_id=%s ssm_id=%s\n", hex.EncodeToString(initResponse.GetRomId()), hex.EncodeToString(initResponse.GetSsmId()))
			for _, param := range supported {
				length := ParameterLength(param)
				units := ""
				if len(param.Conversions) > 0 {
					units = param.Conversions[0].Units
				}
				fmt.Printf("name=%q units=%q address=%s length=%d ecuByteIndex=%d ecuBit=%d supported=%t\n", param.Name, units, param.Address.Address, length, param.EcuByteIndex, param.EcuBit, true)
			}
			return nil
		}

		encoder := json.NewEncoder(os.Stdout)
		for _, param := range allParams {
			length := ParameterLength(param)
			units := ""
			if len(param.Conversions) > 0 {
				units = param.Conversions[0].Units
			}
			out := paramOutput{
				Name:         param.Name,
				Units:        units,
				Address:      param.Address.Address,
				Length:       length,
				EcuByteIndex: param.EcuByteIndex,
				EcuBit:       param.EcuBit,
				Supported:    supportedMap[param.Id],
			}
			if err := encoder.Encode(out); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(paramsCmd)
	paramsCmd.Flags().StringVar(&paramsDefsPath, "defs", "logger_STD_EN_v336.xml", "Path to RomRaider logger definition XML")
	paramsCmd.Flags().StringVar(&paramsFormat, "format", "text", "Output format: text or ndjson")
}
