package vmwareCollectors

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/alarm"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var (
	alarmMap = make(map[string]string)
	alarmMu  sync.RWMutex
)

// FetchAlarms retrieves all alarm definitions and caches their names
func FetchAlarms(ctx context.Context, client *vim25.Client, logger *slog.Logger) error {
	alarmMu.Lock()
	defer alarmMu.Unlock()

	am := alarm.NewManager(client)
	alarms, err := am.GetAlarm(ctx, nil)
	if err != nil {
		return err
	}

	var moAlarms []mo.Alarm
	var alarmRefs []types.ManagedObjectReference
	for _, a := range alarms {
		alarmRefs = append(alarmRefs, a.Self)
	}

	pc := property.DefaultCollector(client)
	err = pc.Retrieve(ctx, alarmRefs, []string{"info"}, &moAlarms)
	if err != nil {
		return err
	}

	for _, a := range moAlarms {
		alarmMap[a.Self.Value] = a.Info.Name
	}

	logger.Debug("msg", fmt.Sprintf("Cached %d alarm definitions", len(alarmMap)), nil)
	return nil
}

// GetAlarmName returns the human-readable name for an alarm MoRef
func GetAlarmName(moRef string) string {
	alarmMu.RLock()
	defer alarmMu.RUnlock()
	if name, ok := alarmMap[moRef]; ok {
		return name
	}
	return moRef
}

// RecordTriggeredAlarms processes the triggered alarm state and sends metrics to the channel
func RecordTriggeredAlarms(ch chan<- prometheus.Metric, namespace, subsystem string, entityRef types.ManagedObjectReference, entityName, vcenter string, triggeredAlarms []types.AlarmState) {
	for _, ta := range triggeredAlarms {
		alarmName := GetAlarmName(ta.Alarm.Value)
		status := ta.OverallStatus

		labels := map[string]string{
			"vcenter":     vcenter,
			"entity_mo":   entityRef.Value,
			"entity_name": entityName,
			"alarm_mo":    ta.Alarm.Value,
			"alarm_name":  alarmName,
			"status":      string(status),
		}

		// Metric value can be 1 (Yellow) or 2 (Red) for overall status
		val := 0.0
		if status == types.ManagedEntityStatusYellow {
			val = 1.0
		} else if status == types.ManagedEntityStatusRed {
			val = 2.0
		} else {
			continue // Don't report Gray or Green if we only want active alerts
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "alarm_triggered"),
				"vCenter Alarm Triggered Status (1 = Yellow, 2 = Red)", nil,
				labels,
			), prometheus.GaugeValue, val,
		)
	}
}
