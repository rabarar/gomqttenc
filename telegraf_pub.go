package main

import (
	"bytes"
	"context"
	"fmt"
	"gomqttenc/parser"
	"gomqttenc/rtl433"
	"gomqttenc/shared"
	"gomqttenc/utils"
	"net/http"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// receive incoming telegraf Messages on the channel and publish to the Telegraf server
func startPublisher(ctx context.Context, wg *sync.WaitGroup, telegrafURL string, telegrafChannel chan shared.TelegrafChannelMessage) {
	defer wg.Done()

	for {

		select {
		case msg := <-telegrafChannel:
			timestamp := time.Now().UnixNano()
			var line string

			switch metric := msg.(type) {
			case parser.NodeInfoMessage:
				if len(metric.PublicKey) > 0 {
					line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=NODEINFO_APP "+
						"id=\"%s\",long_name=\"%s\",short_name=\"%s\",macaddr=\"%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x\",hw_model=\"%s\",public_key=\"֡%x\"",
						metric.Envelope.Device, metric.Id, metric.LongName, metric.ShortName,
						metric.MACaddr[0], metric.MACaddr[1], metric.MACaddr[2], metric.MACaddr[3], metric.MACaddr[4], metric.MACaddr[5],
						metric.HWModel, metric.PublicKey[0])
				} else {
					line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=NODEINFO_APP "+
						"id=\"%s\",long_name=\"%s\",short_name=\"%s\",macaddr=\"%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x:%-2.2x\",hw_model=\"%s\"",
						metric.Envelope.Device, metric.Id, metric.LongName, metric.ShortName,
						metric.MACaddr[0], metric.MACaddr[1], metric.MACaddr[2], metric.MACaddr[3], metric.MACaddr[4], metric.MACaddr[5],
						metric.HWModel)
				}

			case parser.DeviceMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"battery_level=%d,voltage=%f,channel_utilization=%f,air_util_tx=%f,uptime_seconds=%d %d",
					metric.Envelope.Device, metric.BatteryLevel, metric.Voltage,
					metric.ChannelUtilization, metric.AirUtilTx, metric.UptimeSeconds, timestamp)

			case parser.EnvironmentMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"temperature=%f,relative_humidity=%f %d",
					metric.Envelope.Device, metric.Temperature, metric.RelativeHumidity, timestamp)

			case parser.MapReportMessage:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=MAP_REPORT_APP "+
					"long_name=\"%s\",short_name=\"%s\",HwModel=\"%s\",FirmwareVersion=\"%s\",Region=\"%s\",HasDefaultChannel=%t,LatitudeI=%d,LongitudeI=%d,Altitude=%d,PositionPrecision=%d,NumOnlineLocalNodes=%d %d",
					metric.Envelope.Device, metric.LongName, metric.ShortName, metric.HwModel, metric.FirmwareVersion,
					metric.Region, metric.HasDefaultChannel, metric.LatitudeI, metric.LongitudeI, metric.Altitude,
					metric.PositionPrecision, metric.NumOnlineLocalNodes, timestamp)

			case parser.PositionMessage:
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

			case rtl433.RTL433SensorData:
				line = fmt.Sprintf("rtl_433,model=\"%s\",ID=%d "+
					"TemperatureC=%f,Humidity=%d,BatteryOK=%d,Status=%d,MIC=\"%s\" %d",
					metric.Model, metric.ID, metric.TemperatureC, metric.Humidity, metric.BatteryOK, metric.Status, metric.MIC, timestamp)

			case parser.DeepwoodBLE:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", parser.DeepwoodBLEType, metric.MACAddr, parser.ALERT_DETECTED, timestamp)

			case parser.DeepwoodWIFI:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", parser.DeepwoodWIFIType, metric.MACAddr, parser.ALERT_DETECTED, timestamp)

			case parser.DeepwoodProbe:
				line = fmt.Sprintf("Intrusion,type=\"%s\",MAC=\"%s\" alert=\"%s\" %d", parser.DeepwoodProbeType, metric.MACAddr, parser.ALERT_DETECTED, timestamp)

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

			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
				log.Infof("metric published to Telegraf: %s", utils.ReplaceBinaryWithHex(line))
			} else {
				log.Warnf("FAILED metric published to Telegraf Line: [%s], StatusCode: %d, Status: %s", utils.ReplaceBinaryWithHex(line), resp.StatusCode, resp.Status)
			}

		case <-ctx.Done():
			log.Info("Publisher received shutdown signal (cancelled).")
			return
		}
	}
}
