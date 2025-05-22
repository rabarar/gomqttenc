package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// receive incoming telegraf Messages on the channel and publish to the Telegraf server
func startPublisher(ctx context.Context, wg *sync.WaitGroup, telegrafURL string, telegrafChannel chan TelegrafChannelMessage) {
	defer wg.Done()

	for {

		select {
		case msg := <-telegrafChannel:
			timestamp := time.Now().UnixNano()
			var line string

			switch metric := msg.(type) {
			case NodeInfoMessage:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=NODEINFO_APP "+
					"id=\"%s\",long_name=\"%s\",short_name=\"%s\",macaddr=\"%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x\",hw_model=\"%s\",public_key=\"֡%x\"",
					metric.Envelope.Device, metric.Id, metric.LongName, metric.ShortName,
					metric.MACaddr[0], metric.MACaddr[1], metric.MACaddr[2], metric.MACaddr[3], metric.MACaddr[4], metric.MACaddr[5],
					metric.HWModel, metric.PublicKey[0])

			case DeviceMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"battery_level=%d,voltage=%f,channel_utilization=%f,air_util_tx=%f,uptime_seconds=%d %d",
					metric.Envelope.Device, metric.BatteryLevel, metric.Voltage,
					metric.ChannelUtilization, metric.AirUtilTx, metric.UptimeSeconds, timestamp)

			case EnvironmentMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"temperature=%f,relative_humidity=%f %d",
					metric.Envelope.Device, metric.Temperature, metric.RelativeHumidity, timestamp)

			case MapReportMessage:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=MAP_REPORT_APP "+
					"long_name=\"%s\",short_name=\"%s\",HwModel=\"%s\",FirmwareVersion=\"%s\",Region=\"%s\",HasDefaultChannel=%t,LatitudeI=%d,LongitudeI=%d,Altitude=%d,PositionPrecision=%d,NumOnlineLocalNodes=%d %d",
					metric.Envelope.Device, metric.LongName, metric.ShortName, metric.HwModel, metric.FirmwareVersion,
					metric.Region, metric.HasDefaultChannel, metric.LatitudeI, metric.LongitudeI, metric.Altitude,
					metric.PositionPrecision, metric.NumOnlineLocalNodes, timestamp)

			case PositionMessage:
				var ts int64
				var seq, sats int

				if metric.Timestamp == nil {
					ts = 0
				} else {
					ts = *metric.Timestamp
				}
				if metric.SeqNumber == nil {
					seq = 0
				} else {
					seq = *metric.SeqNumber
				}
				if metric.SatsInView == nil {
					sats = 0
				} else {
					sats = *metric.SatsInView
				}

				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=POSITION_APP "+
					"LatitudeI=%d,LongitudeI=%d,Altitude=%d,Time=%d,LocationSource=\"%s\",Timestamp=%d,SeqNumber=%d,SatsInView=%d,GroundSpeed=%d,GroundTrack=%d,PrecisionBits=%d %d",
					metric.Envelope.Device,
					metric.LatitudeI, metric.LongitudeI, metric.Altitude, metric.Time, metric.LocationSource, ts, seq, sats, metric.GroundSpeed, metric.GroundTrack, metric.PrecisionBits, timestamp)

			case RTL433SensorData:
				line = fmt.Sprintf("rtl_433,model=\"%s\",ID=%d "+
					"TemperatureC=%f,Humidity=%d,BatteryOK=%d,Status=%d,MIC=\"%s\" %d",
					metric.Model, metric.ID, metric.TemperatureC, metric.Humidity, metric.BatteryOK, metric.Status, metric.MIC, timestamp)

			case DeepwoodBLE:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", DeepwoodBLEType, metric.MACAddr, ALERT_DETECTED, timestamp)

			case DeepwoodWIFI:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", DeepwoodWIFIType, metric.MACAddr, ALERT_DETECTED, timestamp)

			case DeepwoodProbe:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", DeepwoodProbeType, metric.MACAddr, ALERT_DETECTED, timestamp)

			default:
				log.Error("Unknown Telegraf Channel Message Type received -- no message published: %T", msg)
				return
			}

			// create and send request to the telegraf server
			req, err := http.NewRequest("POST", telegrafURL, bytes.NewBuffer([]byte(line)))
			if err != nil {
				log.Error("Error creating request:", err)
				continue
			}
			req.Header.Set("Content-Type", "text/plain")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Error("Error posting to Telegraf:", err)
				continue
			}
			err = resp.Body.Close()
			if err != nil {
				log.Error("Error failed to close Request Body:", err)
			}

			// TODO this isn't it!
			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
				log.Infof("metric published to Telegraf: %s", replaceBinaryWithHex(line))
			} else {
				log.Warnf("FAILED metric published to Telegraf Line: [%s], StatusCode: %d, Status: %s", replaceBinaryWithHex(line), resp.StatusCode, resp.Status)
			}

		case <-ctx.Done():
			log.Info("Publisher received shutdown signal (cancellled).")
			return
		}
	}
}
