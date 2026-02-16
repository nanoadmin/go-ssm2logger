package cmd

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/Knetic/govaluate"
	. "github.com/rgeyer/ssm2logger/ssm2lib"
)

var defaultTelemetryParamNames = []string{
	"Engine Speed",
	"Throttle Opening Angle",
	"Manifold Relative Pressure",
	"Manifold Absolute Pressure",
	"Primary Wastegate Duty Cycle",
	"Mass Airflow",
	"Ignition Timing",
	"Fine Learning Knock Correction",
	"Feedback Knock Correction",
	"A/F Correction #1",
	"A/F Learning #1",
	"Coolant Temperature",
	"Intake Air Temperature",
	"Vehicle Speed",
	"Rear O2 Sensor",
	"Injector Pulse Width",
	"Battery Voltage",
	"Calculated Load",
}

type selectedParamsResult struct {
	Params    []Ssm2Parameter
	Trimmed   bool
	Wanted    int
	SelectedN int
}

type compiledMapping struct {
	ParameterMapping
	ndjsonKey string
	expr      *govaluate.EvaluableExpression
}

func loadLoggerDefinitions(path string) (*Ssm2Logger, error) {
	xmlfile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer xmlfile.Close()

	xmlbytes, err := ioutil.ReadAll(xmlfile)
	if err != nil {
		return nil, err
	}

	logDefs := &Ssm2Logger{}
	err = xml.Unmarshal(xmlbytes, &logDefs)
	if err != nil {
		return nil, err
	}

	return logDefs, nil
}

func getSsmProtocolParameters(logDefs *Ssm2Logger) []Ssm2Parameter {
	for _, proto := range logDefs.Protocols {
		if proto.Id == "SSM" {
			return proto.Parameters
		}
	}
	return []Ssm2Parameter{}
}

func getSupportedParameters(allParams []Ssm2Parameter, capBytes []byte) []Ssm2Parameter {
	supported := make([]Ssm2Parameter, 0, len(allParams))
	for _, param := range allParams {
		if param.EcuByteIndex < uint(len(capBytes)) {
			if (capBytes[param.EcuByteIndex] & (1 << param.EcuBit)) > 0 {
				supported = append(supported, param)
			}
		}
	}
	return supported
}

func splitParamNames(csv string) []string {
	parts := strings.Split(csv, ",")
	retval := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			retval = append(retval, trimmed)
		}
	}
	return retval
}

func selectParameters(supported []Ssm2Parameter, all bool, paramsCsv string, maxAddresses int) (selectedParamsResult, error) {
	result := selectedParamsResult{}
	chosen := []Ssm2Parameter{}

	if all {
		chosen = append(chosen, supported...)
	} else {
		requestedNames := splitParamNames(paramsCsv)
		if len(requestedNames) == 0 {
			requestedNames = defaultTelemetryParamNames
		}
		lookup := make(map[string]Ssm2Parameter, len(supported))
		for _, param := range supported {
			lookup[strings.ToLower(param.Name)] = param
		}
		for _, name := range requestedNames {
			if param, ok := lookup[strings.ToLower(name)]; ok {
				chosen = append(chosen, param)
			}
		}
	}

	requestedAddressCount := 0
	totalWanted := 0
	trimmed := make([]Ssm2Parameter, 0, len(chosen))
	for _, param := range chosen {
		length := ParameterLength(param)
		totalWanted += length
		if maxAddresses > 0 && requestedAddressCount+length > maxAddresses {
			result.Trimmed = true
			continue
		}
		requestedAddressCount += length
		trimmed = append(trimmed, param)
	}

	result.Params = trimmed
	result.Wanted = totalWanted
	result.SelectedN = len(chosen)
	return result, nil
}

func formatHeaderLabel(mapping ParameterMapping) string {
	if mapping.Units == "" {
		return mapping.Name
	}
	return fmt.Sprintf("%s (%s)", mapping.Name, mapping.Units)
}

var ndjsonCleaner = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeNdjsonKey(name string, units string) string {
	base := strings.ToLower(name)
	base = strings.Replace(base, "(", " ", -1)
	base = strings.Replace(base, ")", " ", -1)
	base = ndjsonCleaner.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")

	if units != "" {
		u := strings.ToLower(units)
		u = ndjsonCleaner.ReplaceAllString(u, "_")
		u = strings.Trim(u, "_")
		if u != "" && !strings.HasSuffix(base, "_"+u) {
			base = base + "_" + u
		}
	}
	return base
}

func buildCompiledMappings(mappings []ParameterMapping) ([]compiledMapping, error) {
	compiled := make([]compiledMapping, 0, len(mappings))
	for _, mapping := range mappings {
		exprString := ""
		for _, conversion := range mapping.Param.Conversions {
			if conversion.Units == mapping.Units {
				exprString = conversion.Expr
				break
			}
		}
		if exprString == "" {
			return nil, fmt.Errorf("unable to find conversion for parameter %q with unit %q", mapping.Name, mapping.Units)
		}

		expr, err := govaluate.NewEvaluableExpression(exprString)
		if err != nil {
			return nil, err
		}

		compiled = append(compiled, compiledMapping{
			ParameterMapping: mapping,
			ndjsonKey:        normalizeNdjsonKey(mapping.Name, mapping.Units),
			expr:             expr,
		})
	}

	return compiled, nil
}

func bytesToInt(value []byte) int {
	if len(value) == 4 {
		return int(uint(value[3]) | uint(value[2])<<8 | uint(value[1])<<16 | uint(value[0])<<24)
	}
	if len(value) == 2 {
		return int(uint(value[1]) | uint(value[0])<<8)
	}
	if len(value) == 1 {
		return int(value[0])
	}
	return 0
}

func evaluateCompiledMapping(mapping compiledMapping, payload []byte) (float64, error) {
	start := mapping.Start
	end := start + mapping.Length
	value := payload[start:end]
	result, err := mapping.expr.Evaluate(map[string]interface{}{"x": bytesToInt(value)})
	if err != nil {
		return 0, err
	}
	return result.(float64), nil
}
