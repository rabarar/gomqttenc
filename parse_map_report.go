package main

import (
	"fmt"
	"regexp"
	"strconv"
)

type MapReportMessage struct {
	LongName            string
	ShortName           string
	HwModel             string
	FirmwareVersion     string
	Region              string
	HasDefaultChannel   bool
	LatitudeI           int
	LongitudeI          int
	Altitude            int
	PositionPrecision   int
	NumOnlineLocalNodes int
}

func parseMapReportMessage(msg string) (*MapReportMessage, error) {
	re := regexp.MustCompile(`long_name:"([^"]+)"\s+short_name:"([^"]+)"\s+hw_model:([^\s]+)\s+firmware_version:"([^"]+)"\s+region:([^\s]+)\s+has_default_channel:(true|false)\s+latitude_i:(-?\d+)\s+longitude_i:(-?\d+)\s+altitude:(\d+)\s+position_precision:(\d+)\s+num_online_local_nodes:(\d+)`)
	match := re.FindStringSubmatch(msg)
	if len(match) != 12 {
		return nil, fmt.Errorf("failed to parse MAP_REPORT message")
	}

	atoi := func(s string) int {
		i, _ := strconv.Atoi(s)
		return i
	}

	parseBool := func(s string) bool {
		return s == "true"
	}

	return &MapReportMessage{
		LongName:            match[1],
		ShortName:           match[2],
		HwModel:             match[3],
		FirmwareVersion:     match[4],
		Region:              match[5],
		HasDefaultChannel:   parseBool(match[6]),
		LatitudeI:           atoi(match[7]),
		LongitudeI:          atoi(match[8]),
		Altitude:            atoi(match[9]),
		PositionPrecision:   atoi(match[10]),
		NumOnlineLocalNodes: atoi(match[11]),
	}, nil
}
