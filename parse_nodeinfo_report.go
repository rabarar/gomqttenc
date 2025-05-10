package main

import (
	"fmt"
	"regexp"
	"strconv"
)

type NodeInfoMessage struct {
	Envelope  MessageEnvelope
	Id        string
	LongName  string
	ShortName string
	MACaddr   []byte
	HWModel   string
	PublicKey []byte
}

func unescapeBytes(escaped string) ([]byte, error) {
	var result []byte
	for i := 0; i < len(escaped); {
		if escaped[i] == '\\' && i+1 < len(escaped) {
			if escaped[i+1] == 'x' && i+3 < len(escaped) {
				hexByte := escaped[i+2 : i+4]
				b, err := strconv.ParseUint(hexByte, 16, 8)
				if err != nil {
					return nil, err
				}
				result = append(result, byte(b))
				i += 4
			} else {
				// handle other escapes if needed
				i++
			}
		} else {
			result = append(result, escaped[i])
			i++
		}
	}
	return result, nil
}

func parseNodeInfoMessage(input string) (*NodeInfoMessage, error) {
	var nodeInfoRegex = regexp.MustCompile(
		`id:"(?P<id>.*?)"\s+long_name:"(?P<long_name>.*?)"\s+short_name:"(?P<short_name>.*?)"\s+macaddr:"(?P<macaddr>.*?)"\s+hw_model:(?P<hw_model>\S+)\s+public_key:"(?P<public_key>.*?)"`)

	match := nodeInfoRegex.FindStringSubmatch(input)
	if match == nil {
		return nil, fmt.Errorf("no match found")
	}

	groupNames := nodeInfoRegex.SubexpNames()
	data := map[string]string{}
	for i, name := range groupNames {
		if i != 0 && name != "" {
			data[name] = match[i]
		}
	}

	macaddr, err := unescapeBytes(data["macaddr"])
	if err != nil {
		return nil, fmt.Errorf("invalid macaddr: %w", err)
	}
	pubkey, err := unescapeBytes(data["public_key"])
	if err != nil {
		return nil, fmt.Errorf("invalid public_key: %w", err)
	}

	return &NodeInfoMessage{
		Id:        data["id"],
		LongName:  data["long_name"],
		ShortName: data["short_name"],
		MACaddr:   macaddr,
		HWModel:   data["hw_model"],
		PublicKey: pubkey,
	}, nil
}
