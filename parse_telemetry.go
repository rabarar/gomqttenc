package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type TelemetryType string

const (
	DeviceMetricsType      TelemetryType = "device_metrics"
	EnvironmentMetricsType TelemetryType = "environment_metrics"
)

type TelemetryMessage struct {
	Time   int64
	Type   TelemetryType
	Parsed interface{}
}

type DeviceMetrics struct {
	Envelope           MessageEnvelope
	BatteryLevel       int
	Voltage            float64
	ChannelUtilization float64
	AirUtilTx          float64
	UptimeSeconds      int
}

type EnvironmentMetrics struct {
	Envelope         MessageEnvelope
	Temperature      float64
	RelativeHumidity float64
}

func parseTelemetryMessage(msg string) (*TelemetryMessage, error) {
	var tm TelemetryMessage

	// Extract timestamp
	timeRe := regexp.MustCompile(`time:(\d+)`)
	timeMatch := timeRe.FindStringSubmatch(msg)
	if len(timeMatch) < 2 {
		return nil, fmt.Errorf("time field missing or invalid")
	}

	timestamp, err := strconv.ParseInt(timeMatch[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}
	tm.Time = timestamp

	switch {
	case strings.Contains(msg, "device_metrics:{"):
		tm.Type = DeviceMetricsType
		metrics := DeviceMetrics{}
		// re := regexp.MustCompile(`battery_level:(\d+) voltage:([\d.]+) channel_utilization:([\d.]+) air_util_tx:([\d.]+) uptime_seconds:(\d+)`)
		re := regexp.MustCompile(`battery_level:(\d+)\s+voltage:([\d.]+)\s+channel_utilization:([\d.]+)\s+air_util_tx:([\d.]+)\s+uptime_seconds:(\d+)`)

		match := re.FindStringSubmatch(msg)
		if len(match) != 6 {
			return nil, fmt.Errorf("failed to parse device_metrics")
		}
		metrics.BatteryLevel, _ = strconv.Atoi(match[1])
		metrics.Voltage, _ = strconv.ParseFloat(match[2], 64)
		metrics.ChannelUtilization, _ = strconv.ParseFloat(match[3], 64)
		metrics.AirUtilTx, _ = strconv.ParseFloat(match[4], 64)
		metrics.UptimeSeconds, _ = strconv.Atoi(match[5])
		tm.Parsed = metrics

	case strings.Contains(msg, "environment_metrics:{"):
		tm.Type = EnvironmentMetricsType
		metrics := EnvironmentMetrics{}
		re := regexp.MustCompile(`temperature:([\d.]+)\s+relative_humidity:([\d.]+)`)
		match := re.FindStringSubmatch(msg)
		if len(match) != 3 {
			return nil, fmt.Errorf("failed to parse environment_metrics")
		}
		metrics.Temperature, _ = strconv.ParseFloat(match[1], 64)
		metrics.RelativeHumidity, _ = strconv.ParseFloat(match[2], 64)
		tm.Parsed = metrics

	default:
		return nil, fmt.Errorf("unknown telemetry format")
	}

	return &tm, nil
}
