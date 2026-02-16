package ssm2lib

import "fmt"

type ParameterMapping struct {
	Param  Ssm2Parameter
	Name   string
	Units  string
	Start  int
	Length int
}

func ParameterLength(param Ssm2Parameter) int {
	if param.Address.Length > 1 {
		return param.Address.Length
	}
	return 1
}

func ExpandAddress(base []byte, offset int) ([]byte, error) {
	if len(base) != 3 {
		return nil, fmt.Errorf("address must be exactly 3 bytes, got %d", len(base))
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	value := int(base[0])<<16 | int(base[1])<<8 | int(base[2])
	value += offset
	if value > 0xFFFFFF {
		return nil, fmt.Errorf("address overflow while adding offset %d", offset)
	}

	return []byte{byte(value >> 16), byte((value >> 8) & 0xFF), byte(value & 0xFF)}, nil
}

func BuildParameterAddressRequest(params []Ssm2Parameter) ([][]byte, []ParameterMapping, error) {
	addresses := [][]byte{}
	mappings := []ParameterMapping{}
	offset := 0

	for _, param := range params {
		base, err := param.Address.GetAddressBytes()
		if err != nil {
			return nil, nil, err
		}
		length := ParameterLength(param)
		for i := 0; i < length; i++ {
			addr, err := ExpandAddress(base, i)
			if err != nil {
				return nil, nil, err
			}
			addresses = append(addresses, addr)
		}

		unit := ""
		if len(param.Conversions) > 0 {
			unit = param.Conversions[0].Units
		}
		mappings = append(mappings, ParameterMapping{
			Param:  param,
			Name:   param.Name,
			Units:  unit,
			Start:  offset,
			Length: length,
		})
		offset += length
	}

	return addresses, mappings, nil
}
