package rtl433

import "errors"

var (
	ErrHandleRTL433Data = errors.New("failed to handle RTL433 Topc")
)

// Define the struct
type RTL433SensorData struct {
	Time         string  `json:"time"`
	Model        string  `json:"model"`
	ID           int     `json:"id"`
	BatteryOK    int     `json:"battery_ok"`
	TemperatureC float64 `json:"temperature_C"`
	Humidity     int     `json:"humidity"`
	Status       int     `json:"status"`
	MIC          string  `json:"mic"`
}
