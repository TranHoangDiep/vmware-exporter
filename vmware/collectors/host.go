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
	} //Common or generic counters that need not be instanced
	iHostCounters = []string{"net.bytesRx.average", "net.bytesTx.average", "net.errorsRx.summation", "net.errorsTx.summation", "net.droppedRx.summation", "net.droppedTx.summation",
		"datastore.read.average", "datastore.write.average", "datastore.numberReadAveraged.average",
		"datastore.numberWriteAveraged.average", "datastore.totalReadLatency.average", "datastore.totalWriteLatency.average",
	} //Counters that come in multiple instances

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

	err := fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"HostSystem"}, []string{"parent", "summary", "runtime", "config"}, &hosts, c.logger,
	)
	if err != nil {
		return err

	}

	wg := sync.WaitGroup{}

	for _, host := range hosts {
		c.logger.Debug("msg", "Processing host", "name", host.Summary.Config.Name, "powerState", host.Runtime.PowerState)
		hostNames[host.Self.Value] = host.Summary.Config.Name

		// 1. Basic Info & Hardware (Exported for ALL hosts)
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

		hostLabels := map[string]string{"hostmo": host.Self.Value, "host": host.Summary.Config.Name, "vcenter": loginData["target"].(string)}

		// 2. Power State Metric (Crucial for Down detection)
		powerState := 0.0
		switch host.Runtime.PowerState {
		case "poweredOn":
			powerState = 1.0
		case "standBy":
			powerState = 2.0
		default:
			powerState = 0.0 // poweredOff or unknown
		}

		c.logger.Info("msg", "Exporting power_state", "host", host.Summary.Config.Name, "value", powerState)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, hostSubsystem, "power_state"),
				"Host power state (1=poweredOn, 0=poweredOff, 2=standBy)",
				[]string{"hostmo", "host", "vcenter"}, nil,
			),
			prometheus.GaugeValue,
			powerState,
			host.Self.Value, host.Summary.Config.Name, loginData["target"].(string),
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

		// 3. Conditional Metrics (Only for Powered On & Connected Hosts)
		if host.Runtime.PowerState == "poweredOn" && host.Runtime.ConnectionState == "connected" && !host.Runtime.InMaintenanceMode {

			hostRefs = append(hostRefs, host.Self)

			c.logger.Debug("msg", fmt.Sprintf("gathering metrics for host %s with moRef %s\n", host.Summary.Config.Name, host.Self.Value), nil)

			// Physical NIC Link Status
			if host.Config != nil && host.Config.Network != nil {
				for _, pnic := range host.Config.Network.Pnic {
					linkStatus := 0.0
					speed := "Unknown"
					var speedValue float64 = 0
					if pnic.LinkSpeed != nil {
						linkStatus = 1.0
						speedValue = float64(pnic.LinkSpeed.SpeedMb)
						speed = fmt.Sprintf("%d Mbps", pnic.LinkSpeed.SpeedMb)
					}
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, hostSubsystem, "nic_link_status"),
							"Physical NIC link status (1=Up, 0=Down)", nil,
							map[string]string{
								"hostmo":  host.Self.Value,
								"host":    host.Summary.Config.Name,
								"device":  pnic.Device,
								"vcenter": loginData["target"].(string),
								"speed":   speed,
							},
						), prometheus.GaugeValue, linkStatus,
					)
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, hostSubsystem, "nic_speed_mbps"),
							"Physical NIC speed in Mbps", nil,
							map[string]string{
								"hostmo":  host.Self.Value,
								"host":    host.Summary.Config.Name,
								"device":  pnic.Device,
								"vcenter": loginData["target"].(string),
							},
						), prometheus.GaugeValue, speedValue,
					)
				}
			}

			// PortGroup VLAN IDs
			if host.Config != nil && host.Config.Network != nil {
				for _, pg := range host.Config.Network.Portgroup {
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc(
							prometheus.BuildFQName(namespace, hostSubsystem, "portgroup_vlan"),
							"Host PortGroup VLAN ID", nil,
							map[string]string{
								"hostmo":    host.Self.Value,
								"host":      host.Summary.Config.Name,
								"portgroup": pg.Spec.Name,
								"vswitch":   pg.Spec.VswitchName,
								"vcenter":   loginData["target"].(string),
							},
						), prometheus.GaugeValue, float64(pg.Spec.VlanId),
					)
				}
			}

			// FC HBA Link Status
			if host.Config != nil && host.Config.StorageDevice != nil {
				for _, hba := range host.Config.StorageDevice.HostBusAdapter {
					if fcHba, ok := hba.(*types.HostFibreChannelHba); ok {
						linkStatus := 0.0
						if fcHba.Status == "online" {
							linkStatus = 1.0
						}
						wwn := fmt.Sprintf("%016x", fcHba.PortWorldWideName)
						ch <- prometheus.MustNewConstMetric(
							prometheus.NewDesc(
								prometheus.BuildFQName(namespace, hostSubsystem, "fc_link_status"),
								"Fibre Channel HBA link status (1=Online, 0=Offline)", nil,
								map[string]string{
									"hostmo":  host.Self.Value,
									"host":    host.Summary.Config.Name,
									"device":  fcHba.Device,
									"vcenter": loginData["target"].(string),
									"wwn":     wwn,
									"status":  fcHba.Status,
								},
							), prometheus.GaugeValue, linkStatus,
						)
					}
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
	return nil
}
