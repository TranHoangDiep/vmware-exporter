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
	hostSubsystem = "host"
)

var hostCollectorFlag = flag.Bool(fmt.Sprintf("collector.%s", hostSubsystem), collector.DefaultEnabled, fmt.Sprintf("Enable the %s collector (default: %v)", hostSubsystem, collector.DefaultEnabled))

var (
	cHostCounters = []string{"cpu.usagemhz.average", "cpu.demand.average", "cpu.latency.average", "cpu.entitlement.latest",
		"cpu.ready.summation", "cpu.readiness.average", "cpu.costop.summation", "cpu.maxlimited.summation",
		"mem.entitlement.average", "mem.active.average", "mem.shared.average", "mem.vmmemctl.average",
		"mem.swapped.average", "mem.consumed.average", "sys.uptime.latest",
		"power.power.average", "power.energy.summation",
		"sys.resourceCpuUsage.average", "sys.resourceCpuMaxLimited.average", "sys.resourceCpuRunWait.average",
		"sys.resourceMemAllocated.latest", "sys.resourceMemConsumed.latest", "sys.resourceMemCow.latest",
		"rescpu.actav.latest", "rescpu.maxlimited.latest", "rescpu.runav.latest",
	} //Common or generic counters that need not be instanced
	iHostCounters = []string{"net.bytesRx.average", "net.bytesTx.average", "net.errorsRx.summation", "net.errorsTx.summation", "net.droppedRx.summation", "net.droppedTx.summation",
		"net.packetsRx.summation", "net.packetsTx.summation", "net.broadcastRx.summation", "net.multicastRx.summation",
		"disk.commands.summation", "disk.numberRead.summation", "disk.numberWrite.summation",
		"disk.kernelReadLatency.average", "disk.kernelWriteLatency.average",
		"storagePath.totalReadLatency.average", "storagePath.totalWriteLatency.average",
		"storageAdapter.totalReadLatency.average",
		"datastore.read.average", "datastore.write.average", "datastore.numberReadAveraged.average",
		"datastore.numberWriteAveraged.average", "datastore.totalReadLatency.average", "datastore.totalWriteLatency.average",
	} //Counters that come in multiple i

)

type hostCollector struct {
	logger *slog.Logger
}

func init() {
	collector.RegisterCollector("host", hostCollectorFlag, NewhostCollector)
}

func NewhostCollector(logger *slog.Logger) (collector.Collector, error) {
	return &hostCollector{logger}, nil
}

func (c *hostCollector) Update(ch chan<- prometheus.Metric, namespace string, clientAPI collector.ClientAPI, loginData map[string]interface{}, params map[string]string) error {

	var (
		hosts     []mo.HostSystem
		hostRefs  []types.ManagedObjectReference
		hostNames = make(map[string]string)
	)

	begin := time.Now()

	// Fetch Alarms first to cache names
	FetchAlarms(loginData["ctx"].(context.Context), loginData["client"].(*vim25.Client), c.logger)

	err := fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"HostSystem"}, []string{"parent", "summary", "runtime", "triggeredAlarmState", "config.network", "config.storageDevice"}, &hosts, c.logger,
	)
	if err != nil {
		return err

	}

	wg := sync.WaitGroup{}

	for _, host := range hosts {

		hostLabels := map[string]string{
			"hostmo":  host.Self.Value,
			"host":    host.Summary.Config.Name,
			"vcenter": loginData["target"].(string),
		}

		// Power State metric for ALL hosts (1 = poweredOn, 0 = poweredOff, 2 = standBy)
		powerState := 0.0
		if host.Runtime.PowerState == "poweredOn" {
			powerState = 1.0
		} else if host.Runtime.PowerState == "standBy" {
			powerState = 2.0
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, hostSubsystem, "power_state"),
				"Host power state (1 = poweredOn, 0 = poweredOff, 2 = standBy)", nil,
				hostLabels,
			), prometheus.GaugeValue, powerState,
		)

		// Only collect detailed metrics for powered-on, connected hosts
		if host.Runtime.PowerState == "poweredOn" && host.Runtime.ConnectionState == "connected" && !host.Runtime.InMaintenanceMode {

			hostRefs = append(hostRefs, host.Self)

			hostNames[host.Self.Value] = host.Summary.Config.Name

			c.logger.Debug("msg", fmt.Sprintf("gathering metrics for host %s with moRef %s\n", host.Summary.Config.Name, host.Self.Value), nil)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "info"),
					"Basic host info", nil,
					map[string]string{"hostmo": host.Self.Value, "host": host.Summary.Config.Name, "cmo": host.Parent.Value,
						"vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, 1.0,
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "hardware_info"),
					"Hardware information", nil,
					map[string]string{"hostmo": host.Self.Value, "host": host.Summary.Config.Name, "vendor": host.Summary.Hardware.Vendor,
						"model": host.Summary.Hardware.Model, "cpu_type": host.Summary.Hardware.CpuModel, "vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, 1.0,
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "software_info"),
					"Software Information", nil,
					map[string]string{"hostmo": host.Self.Value, "host": host.Summary.Config.Name, "software": host.Summary.Config.Product.Name,
						"version": host.Summary.Config.Product.Version, "build": host.Summary.Config.Product.Build,
						"vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, 1.0,
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "cpu_corecount"),
					"Number of physical CPU cores", nil, hostLabels,
				), prometheus.GaugeValue, float64(host.Summary.Hardware.NumCpuCores),
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "cpu_threadcount"),
					"Number of virtual (HT) CPU cores", nil, hostLabels,
				), prometheus.GaugeValue, float64(host.Summary.Hardware.NumCpuThreads),
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "cpu_capacity"),
					"Average CPU Frequency", nil, hostLabels,
				), prometheus.GaugeValue, float64(host.Summary.Hardware.CpuMhz),
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, hostSubsystem, "mem_capacity"),
					"Amount of RAM in MB", nil, hostLabels,
				), prometheus.GaugeValue, float64(host.Summary.Hardware.MemorySize),
			)

			// New Metric: vCenter Alarms
			if host.TriggeredAlarmState != nil {
				RecordTriggeredAlarms(ch, namespace, hostSubsystem, host.Self, host.Summary.Config.Name, loginData["target"].(string), host.TriggeredAlarmState)
			}

			// --- NIC Link Status Metrics ---
			if host.Config != nil && host.Config.Network != nil && host.Config.Network.Pnic != nil {
				for _, pnic := range host.Config.Network.Pnic {
					linkUp := 0.0
					if pnic.LinkSpeed != nil {
						linkUp = 1.0
					}
					nicLabels := map[string]string{
						"hostmo":  host.Self.Value,
						"host":    host.Summary.Config.Name,
						"vcenter": loginData["target"].(string),
						"device":  pnic.Device,
					}

					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, hostSubsystem, "nic_link_status"),
							"Physical NIC link status (1 = Up, 0 = Down)", nil,
							nicLabels,
						), prometheus.GaugeValue, linkUp,
					)

					// NIC Speed in Mbps
					if pnic.LinkSpeed != nil {
						ch <- prometheus.MustNewConstMetric(
							prometheus.NewDesc(
								prometheus.BuildFQName(namespace, hostSubsystem, "nic_speed_mbps"),
								"Physical NIC link speed in Mbps", nil,
								nicLabels,
							), prometheus.GaugeValue, float64(pnic.LinkSpeed.SpeedMb),
						)
					}
				}
			}

			// --- FC HBA Link Status Metrics ---
			if host.Config != nil && host.Config.StorageDevice != nil && host.Config.StorageDevice.HostBusAdapter != nil {
				for _, hbaBase := range host.Config.StorageDevice.HostBusAdapter {
					// Check if it's a FibreChannel HBA
					if fcHba, ok := hbaBase.(*types.HostFibreChannelHba); ok {
						fcStatus := 0.0
						if fcHba.Status == "online" {
							fcStatus = 1.0
						}
						fcLabels := map[string]string{
							"hostmo":  host.Self.Value,
							"host":    host.Summary.Config.Name,
							"vcenter": loginData["target"].(string),
							"device":  fcHba.Device,
							"wwn":     fmt.Sprintf("%016x", fcHba.PortWorldWideName),
							"status":  string(fcHba.Status),
						}
						ch <- prometheus.MustNewConstMetric(
							prometheus.NewDesc(
								prometheus.BuildFQName(namespace, hostSubsystem, "fc_link_status"),
								"Fibre Channel HBA link status (1 = Online, 0 = Offline)", nil,
								fcLabels,
							), prometheus.GaugeValue, fcStatus,
						)
					}
				}
			}

			// --- PortGroup VLAN Metrics ---
			if host.Config != nil && host.Config.Network != nil && host.Config.Network.Portgroup != nil {
				for _, pg := range host.Config.Network.Portgroup {
					pgLabels := map[string]string{
						"hostmo":    host.Self.Value,
						"host":      host.Summary.Config.Name,
						"vcenter":   loginData["target"].(string),
						"portgroup": pg.Spec.Name,
						"vswitch":   pg.Spec.VswitchName,
					}
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, hostSubsystem, "portgroup_vlan"),
							"PortGroup VLAN ID (1-4094, 4095 = Trunk, 0 = None)", nil,
							pgLabels,
						), prometheus.GaugeValue, float64(pg.Spec.VlanId),
					)
				}
			}

		}
	}

	c.logger.Debug("msg", fmt.Sprintf("Time to process PropColletor for Host: %f\n", time.Since(begin).Seconds()), nil)

	c.logger.Debug("msg", fmt.Sprintf("Max samples set to %d\n", loginData["samples"].(int32)), nil)

	begin = time.Now()

	if len(hostRefs) > 0 {

		wg.Add(2)
		for i := 0; i < 2; i++ {
			switch {
			case i == 0:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "HostSystem", namespace, hostSubsystem, "", cHostCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), hostRefs, hostNames)
					wg.Done()
				}(i)

			case i == 1:
				go func(i int) {
					scrapePerformance(loginData["ctx"].(context.Context), ch, c.logger, loginData["samples"].(int32), loginData["interval"].(int32), loginData["perf"].(*performance.Manager),
						loginData["target"].(string), "HostSystem", namespace, hostSubsystem, "*", iHostCounters,
						loginData["counters"].(map[string]*types.PerfCounterInfo), hostRefs, hostNames)
					wg.Done()
				}(i)
			}

		}

		wg.Wait()
	}
	c.logger.Debug("msg", fmt.Sprintf("Time to process PerfMan for Host: %f\n", time.Since(begin).Seconds()), nil)

	return nil
}
