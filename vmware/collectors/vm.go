package vmwareCollectors

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prezhdarov/prometheus-exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	vmSubsystem = "vm"
)

var vmCollectorFlag = flag.Bool(fmt.Sprintf("collector.%s", vmSubsystem), collector.DefaultEnabled, fmt.Sprintf("Enable the %s collector (default: %v)", vmSubsystem, collector.DefaultEnabled))

var (
	cVMCounters = []string{"cpu.usagemhz.average", "cpu.demand.average", "cpu.latency.average", "cpu.entitlement.latest",
		"cpu.ready.summation", "cpu.readiness.average", "cpu.costop.summation", "cpu.maxlimited.summation",
		"mem.entitlement.average", "mem.active.average", "mem.shared.average", "mem.vmmemctl.average",
		"mem.swapped.average", "mem.consumed.average", "sys.uptime.latest",
	} //Common or generic counters that need not be instanced
	iVMCounters = []string{"net.bytesRx.average", "net.bytesTx.average",
		"datastore.read.average", "datastore.write.average", "datastore.numberReadAveraged.average",
		"datastore.numberWriteAveraged.average", "datastore.totalReadLatency.average", "datastore.totalWriteLatency.average"} //Counters that come in multiple i
)

type vmCollector struct {
	logger *slog.Logger
}

func init() {
	collector.RegisterCollector(vmSubsystem, vmCollectorFlag, NewvmCollector)
}

// NewMeminfoCollector returns a new Collector exposing memory stats.
func NewvmCollector(logger *slog.Logger) (collector.Collector, error) {
	return &vmCollector{logger}, nil
}

func (c *vmCollector) Update(ch chan<- prometheus.Metric, namespace string, clientAPI collector.ClientAPI, loginData map[string]interface{}, params map[string]string) error {

	var (
		vms     []mo.VirtualMachine
		vmRefs  []types.ManagedObjectReference
		vmNames = make(map[string]string)
	)

	begin := time.Now()

	err := fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"VirtualMachine"}, []string{"summary", "runtime", "storage", "guest", "snapshot"}, &vms, c.logger,
	)
	if err != nil {
		return err

	}

	wg := sync.WaitGroup{}

	for _, vm := range vms {
		vmNames[vm.Self.Value] = vm.Summary.Config.Name

		powerStateValue := 0.0
		if vm.Runtime.PowerState == "poweredOn" {
			powerStateValue = 1.0
			vmRefs = append(vmRefs, vm.Self)
		}

		// Metadata & Power State (ALL VMs)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "info"),
				"Basic VM metadata", nil,
				map[string]string{
					"vmmo":        vm.Self.Value,
					"vm":          vm.Summary.Config.Name,
					"hostmo":      vm.Runtime.Host.Value,
					"vcenter":     loginData["target"].(string),
					"guest_os":    vm.Summary.Config.GuestFullName,
					"power_state": string(vm.Runtime.PowerState),
				},
			), prometheus.GaugeValue, 1.0,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "power_state"),
				"VM power state (1=poweredOn, 0=poweredOff/other)", nil,
				map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
			), prometheus.GaugeValue, powerStateValue,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "cpu_corecount"),
				"Number of virtual CPUs", nil, map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
			), prometheus.GaugeValue, float64(vm.Summary.Config.NumCpu),
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "mem_capacity"),
				"Virtual memory configured in MB", nil, map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
			), prometheus.GaugeValue, float64(vm.Summary.Config.MemorySizeMB),
		)

		// 3. Conditional Metrics (Only for Powered On VMs)
		if vm.Runtime.PowerState == "poweredOn" {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, vmSubsystem, "tools_running_status"),
					"VMware Tools running status (1 = running, 0 = not running)", nil,
					map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, toolsStatus,
			)

			// New Metric: VM Uptime
			if vm.Summary.QuickStats.UptimeSeconds != 0 {
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, vmSubsystem, "uptime_seconds"),
						"VM uptime in seconds", nil,
						map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(vm.Summary.QuickStats.UptimeSeconds),
				)
			}

			// New Metric: Snapshot Count & Age
			snapshotCount := 0.0
			snapshotAgeDays := 0.0
			if vm.Snapshot != nil && vm.Snapshot.RootSnapshotList != nil {
				var oldest time.Time
				snapshotCount = getSnapshotInfo(vm.Snapshot.RootSnapshotList, &oldest)
				if !oldest.IsZero() {
					snapshotAgeDays = time.Since(oldest).Hours() / 24.0
				}
			}
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, vmSubsystem, "snapshot_count"),
					"Number of snapshots", nil,
					map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, snapshotCount,
			)

			if snapshotCount > 0 {
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, vmSubsystem, "snapshot_age_days"),
						"Age of the oldest snapshot in days", nil,
						map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name, "hostmo": vm.Runtime.Host.Value, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, snapshotAgeDays,
				)
			}

			// New Metric: VM Network Info (IPs and MACs)
			if vm.Guest != nil {
				for _, nic := range vm.Guest.Net {
					ip := "-"
					if len(nic.IpAddress) > 0 {
						ip = nic.IpAddress[0]
					}
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, vmSubsystem, "network_info"),
							"VM network interface info", nil,
							map[string]string{
								"vmmo":        vm.Self.Value,
								"vm":          vm.Summary.Config.Name,
								"hostmo":      vm.Runtime.Host.Value,
								"vcenter":     loginData["target"].(string),
								"network":     nic.Network,
								"mac_address":  nic.MacAddress,
								"ip_address":   ip,
								"connected":   fmt.Sprintf("%t", nic.Connected),
							},
						), prometheus.GaugeValue, 1.0,
					)
				}
			}

			for _, datastore := range vm.Storage.PerDatastoreUsage {

				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, vmSubsystem, "datastore_capacity_used"),
						"Virtual memory configured in MB", nil,
						map[string]string{"vmmo": vm.Self.Value, "vm": vm.Summary.Config.Name,
							"vcenter": loginData["target"].(string), "dsmo": datastore.Datastore.Value},
					), prometheus.GaugeValue, float64(datastore.Committed),
				)
			}
		}

	}

	c.logger.Debug("msg", fmt.Sprintf("Time to process PropColletor for VM: %f\n", time.Since(begin).Seconds()), nil)

	begin = time.Now()

	if len(vmRefs) > 0 {

		wg.Add(2)
		for i := 0; i < 2; i++ {
			switch {
			case i == 0:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "VirtualMachine", namespace, vmSubsystem, "", cVMCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), vmRefs, vmNames)
					wg.Done()
				}(i)

			case i == 1:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "VirtualMachine", namespace, vmSubsystem, "*", iVMCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), vmRefs, vmNames)
					wg.Done()
				}(i)
			}

		}

		wg.Wait()

	}

	c.logger.Debug("msg", fmt.Sprintf("Time to process PerfMan for VM: %f\n", time.Since(begin).Seconds()), nil)

	return nil
}

// Helper function to count snapshots and find oldest creation time
func getSnapshotInfo(snapshots []types.VirtualMachineSnapshotTree, oldest *time.Time) float64 {
	count := 0.0
	for _, snap := range snapshots {
		count++
		if oldest.IsZero() || snap.CreateTime.Before(*oldest) {
			*oldest = snap.CreateTime
		}
		if snap.ChildSnapshotList != nil {
			count += getSnapshotInfo(snap.ChildSnapshotList, oldest)
		}
	}
	return count
}
