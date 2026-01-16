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
		"power.power.average", "power.energy.summation",
		"sys.osUptime.latest",
		"rescpu.actav.latest", "rescpu.maxlimited.latest", "rescpu.runav.latest",
		"disk.maxTotalLatency.latest",
	} //Common or generic counters that need not be instanced
	iVMCounters = []string{"net.bytesRx.average", "net.bytesTx.average",
		"net.packetsRx.summation", "net.packetsTx.summation", "net.droppedRx.summation", "net.droppedTx.summation",
		"net.broadcastRx.summation", "net.broadcastTx.summation", "net.multicastRx.summation", "net.multicastTx.summation",
		"disk.commands.summation", "disk.numberRead.summation", "disk.numberWrite.summation",
		"disk.busResets.summation", "disk.commandsAborted.summation",
		"virtualDisk.totalReadLatency.average", "virtualDisk.totalWriteLatency.average",
		"virtualDisk.read.average", "virtualDisk.write.average",
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

	// Fetch Alarms first to cache names
	FetchAlarms(loginData["ctx"].(context.Context), loginData["client"].(*vim25.Client), c.logger)

	err := fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"VirtualMachine"}, []string{"summary", "runtime", "storage", "guest", "snapshot", "layoutEx", "triggeredAlarmState"}, &vms, c.logger,
	)
	if err != nil {
		return err
	}

	// Fetch Hosts and Clusters to resolve names (Host Name & Cluster Name)
	var (
		hosts    []mo.HostSystem
		clusters []mo.ClusterComputeResource
		hostMap  = make(map[string]mo.HostSystem)
		clusterMap = make(map[string]string)
	)

	// Fetch Hosts
	err = fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"HostSystem"}, []string{"summary", "parent"}, &hosts, c.logger,
	)
	if err == nil {
		for _, h := range hosts {
			hostMap[h.Self.Value] = h
		}
	}

	// Fetch Clusters
	err = fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"ClusterComputeResource"}, []string{"name"}, &clusters, c.logger,
	)
	if err == nil {
		for _, cl := range clusters {
			clusterMap[cl.Self.Value] = cl.Name
		}
	}


	wg := sync.WaitGroup{}

	for _, vm := range vms {

		// Collect static info for ALL VMs (even powered off)
		vmRefs = append(vmRefs, vm.Self)
		vmNames[vm.Self.Value] = vm.Summary.Config.Name

		// Resolve Host and Cluster Names
		hostName := ""
		clusterName := ""
		hostID := ""
		if vm.Runtime.Host != nil {
			hostID = vm.Runtime.Host.Value
			if h, ok := hostMap[hostID]; ok {
				hostName = h.Summary.Config.Name
				// Resolve Cluster from Host Parent
				if h.Parent != nil {
					if cName, found := clusterMap[h.Parent.Value]; found {
						clusterName = cName
					}
				}
			}
		}

		// Common Labels
		labels := map[string]string{
			"vmmo":         vm.Self.Value,
			"vm":           vm.Summary.Config.Name,
			"hostmo":       hostID,
			"host_name":    hostName,
			"cluster_name": clusterName,
			"vcenter":      loginData["target"].(string),
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "info"),
				"This is basic vm info to be used for parent reference", nil,
				labels,
			), prometheus.GaugeValue, 1.0,
		)

		// New Metric: Power State
		powerState := 0.0
		if vm.Runtime.PowerState == "poweredOn" {
			powerState = 1.0
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "power_state"),
				"VM Power State (1 = poweredOn, 0 = poweredOff/suspended)", nil,
				labels,
			), prometheus.GaugeValue, powerState,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "cpu_corecount"),
				"Number of virtual CPUs", nil, labels,
			), prometheus.GaugeValue, float64(vm.Summary.Config.NumCpu),
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "mem_capacity"),
				"Virtual memory configured in MB", nil, labels,
			), prometheus.GaugeValue, float64(vm.Summary.Config.MemorySizeMB),
		)

		// New Metric: VMware Tools Status
		toolsStatus := 0.0
		if vm.Guest != nil && vm.Guest.ToolsRunningStatus == "guestToolsRunning" {
			toolsStatus = 1.0
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, vmSubsystem, "tools_running_status"),
				"VMware Tools running status (1 = running, 0 = not running)", nil,
				labels,
			), prometheus.GaugeValue, toolsStatus,
		)

		// Only collect these if powered on or appropriate
		if vm.Runtime.PowerState == "poweredOn" {

			// New Metric: VM Uptime
			if vm.Summary.QuickStats.UptimeSeconds != 0 {
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, vmSubsystem, "uptime_seconds"),
						"VM uptime in seconds", nil,
						labels,
					), prometheus.GaugeValue, float64(vm.Summary.QuickStats.UptimeSeconds),
				)
			}

			// New Metric: Snapshot Count
			snapshotCount := 0.0
			if vm.Snapshot != nil && vm.Snapshot.RootSnapshotList != nil {
				snapshotCount = countSnapshots(vm.Snapshot.RootSnapshotList)
			}
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, vmSubsystem, "snapshot_count"),
					"Number of snapshots", nil,
					labels,
				), prometheus.GaugeValue, snapshotCount,
			)
			// New Metric: Snapshot Info (Names)
			if vm.Snapshot != nil && vm.Snapshot.RootSnapshotList != nil {
				recordSnapshotInfo(ch, namespace, vmSubsystem, labels, vm.Snapshot.RootSnapshotList)
			}

			// New Metric: Snapshot Size in GB
			snapshotSizeGB := 0.0
			if vm.LayoutEx != nil {
				var sizeBytes int64
				fileMap := make(map[int32]types.VirtualMachineFileLayoutExFileInfo)
				for _, f := range vm.LayoutEx.File {
					fileMap[f.Key] = f
				}

				for _, snap := range vm.LayoutEx.Snapshot {
					if f, ok := fileMap[snap.DataKey]; ok {
						sizeBytes += f.Size
					}
					for _, disk := range snap.Disk {
						for _, chain := range disk.Chain {
							for _, fileKey := range chain.FileKey {
								if f, ok := fileMap[fileKey]; ok {
									sizeBytes += f.Size
								}
							}
						}
					}
				}
				snapshotSizeGB = float64(sizeBytes) / (1024 * 1024 * 1024)
			}
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, vmSubsystem, "snapshot_size_gb"),
					"Total size of snapshots in GB", nil,
					labels,
				), prometheus.GaugeValue, snapshotSizeGB,
			)

			// New Metric: vCenter Alarms
			if vm.TriggeredAlarmState != nil {
				RecordTriggeredAlarms(ch, namespace, vmSubsystem, vm.Self, vm.Summary.Config.Name, loginData["target"].(string), vm.TriggeredAlarmState)
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

	// Time to process PropColletor for VM
	c.logger.Debug("msg", fmt.Sprintf("Time to process PropColletor for VM: %f\n", time.Since(begin).Seconds()), nil)

	begin = time.Now()

	// Filter vmRefs for performance scraping (only PoweredOn)
	var perfVmRefs []types.ManagedObjectReference
	for _, vm := range vms {
		if vm.Runtime.PowerState == "poweredOn" {
			perfVmRefs = append(perfVmRefs, vm.Self)
		}
	}

	if len(perfVmRefs) > 0 {

		wg.Add(2)
		for i := 0; i < 2; i++ {
			switch {
			case i == 0:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "VirtualMachine", namespace, vmSubsystem, "", cVMCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), perfVmRefs, vmNames)
					wg.Done()
				}(i)

			case i == 1:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "VirtualMachine", namespace, vmSubsystem, "*", iVMCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), perfVmRefs, vmNames)
					wg.Done()
				}(i)
			}

		}

		wg.Wait()

	}

	c.logger.Debug("msg", fmt.Sprintf("Time to process PerfMan for VM: %f\n", time.Since(begin).Seconds()), nil)

	return nil
}

// Helper function to count snapshots recursively
func countSnapshots(snapshots []types.VirtualMachineSnapshotTree) float64 {
	count := 0.0
	for _, snap := range snapshots {
		count++
		if snap.ChildSnapshotList != nil {
			count += countSnapshots(snap.ChildSnapshotList)
		}
	}
	return count
}
// Helper function to record snapshot info recursively
func recordSnapshotInfo(ch chan<- prometheus.Metric, namespace, subsystem string, commonLabels map[string]string, snapshots []types.VirtualMachineSnapshotTree) {
	for _, snap := range snapshots {
		labels := make(map[string]string)
		for k, v := range commonLabels {
			labels[k] = v
		}
		labels["snapshot_name"] = snap.Name
		labels["snapshot_mo"] = snap.Snapshot.Value

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "snapshot_info"),
				"Snapshot information (names and MoRefs)", nil,
				labels,
			), prometheus.GaugeValue, 1.0,
		)

		if snap.ChildSnapshotList != nil {
			recordSnapshotInfo(ch, namespace, subsystem, commonLabels, snap.ChildSnapshotList)
		}
	}
}
