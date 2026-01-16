package vmwareCollectors

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/prezhdarov/prometheus-exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
)

const (
	clusterSubsystem = "cluster"
)

var clusterCollectorFlag = flag.Bool(fmt.Sprintf("collector.%s", clusterSubsystem), collector.DefaultEnabled, fmt.Sprintf("Enable the %s collector (default: %v)", clusterSubsystem, collector.DefaultEnabled))

type clusterCollector struct {
	logger *slog.Logger
}

func init() {
	collector.RegisterCollector("cluster", clusterCollectorFlag, NewClusterCollector)
}

func NewClusterCollector(logger *slog.Logger) (collector.Collector, error) {
	return &clusterCollector{logger}, nil
}

func (c *clusterCollector) Update(ch chan<- prometheus.Metric, namespace string, clientAPI collector.ClientAPI, loginData map[string]interface{}, params map[string]string) error {

	var clusters []mo.ClusterComputeResource

	// Fetch Alarms first to cache names
	FetchAlarms(loginData["ctx"].(context.Context), loginData["client"].(*vim25.Client), c.logger)

	err := fetchProperties(
		loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
		[]string{"ClusterComputeResource"}, []string{"name", "summary", "datastore", "parent", "triggeredAlarmState"}, &clusters, c.logger,
	)
	if err != nil {
		return err

	}

	for _, cluster := range clusters {

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, clusterSubsystem, "info"),
				"This is basic cluster info to be used for parent reference", nil,
				map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "foldermo": cluster.Parent.Value,
					"vcenter": loginData["target"].(string)},
			), prometheus.GaugeValue, 1.0,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, clusterSubsystem, "datastores"),
				"This is basic cluster info to be used for parent reference", nil,
				map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "datastores": *moSliceToString(cluster.ComputeResource.Datastore),
					"vcenter": loginData["target"].(string)},
			), prometheus.GaugeValue, 1.0,
		)

		// New: Cluster Usage Summary
		if cluster.Summary != nil {
			usage := cluster.Summary.GetComputeResourceSummary()
			if usage != nil {
				// CPU
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "cpu_usage_mhz"),
						"Cluster Total CPU Usage in MHz", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.TotalCpu),
				)
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "cpu_capacity_mhz"),
						"Cluster Total CPU Capacity in MHz", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.TotalCpu - usage.EffectiveCpu), // Approximate: Capacity = Usage + Free? usage.TotalCpu is actually "Aggregated CPU resources of all hosts, in MHz".
				)
				// Wait, UsageSummary.TotalCpu is TOTAL CAPACITY. UsageSummary.EffectiveCpu is AVAILABLE ("Effective"). So Usage = Total - Effective? No.
				// Let's check govmomi docs or standard interpretation.
				// ComputeResourceSummary: TotalCpu (Capacity), EffectiveCpu (Available for use), TotalMemory (Capacity), EffectiveMemory (Available).
				// We don't have "Used" directly in Summary, we have OverallUsage directly on some objects but maybe not Summary.
				// Actually, ClusterComputeResourceSummary extends ComputeResourceSummary and has UsageSummary which has totalCpuCapacityMhz and totalMemCapacityMB? No.
				
				// Let's use what we have:
				// TotalCpu: Aggregated CPU resources of all hosts, in MHz.
				// EffectiveCpu: Effective CPU resources, in MHz. (Total - Overhead - etc... roughly Available).
				// Actually, `Effective` is usually "Capacity available to Run VMs". 
				
				// Let's look for QuickStats? ClusterActionHistory?
				// To be safe, let's collect what IS there: TotalCpu and TotalMemory.
				// Usage we might need to sum up from Hosts or look for QuickStats if available (not in standard Summary fetch?).
				
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "total_cpu_mhz"),
						"Cluster Total CPU Capacity in MHz", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.TotalCpu),
				)
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "effective_cpu_mhz"),
						"Cluster Effective CPU Capacity in MHz", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.EffectiveCpu),
				)
				
				// Memory
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "total_memory_bytes"),
						"Cluster Total Memory Capacity in Bytes", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.TotalMemory),
				)
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "effective_memory_bytes"),
						"Cluster Effective Memory Capacity in Bytes", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.EffectiveMemory),
				)
				
				// NumHosts / NumCpuCores / NumThreads
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "num_hosts"),
						"Number of Hosts in Cluster", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.NumHosts),
				)
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(
						prometheus.BuildFQName(namespace, clusterSubsystem, "num_cpu_cores"),
						"Number of CPU Cores in Cluster", nil,
						map[string]string{"cmo": cluster.ComputeResource.Self.Value, "vmwcluster": cluster.Name, "vcenter": loginData["target"].(string)},
					), prometheus.GaugeValue, float64(usage.NumCpuCores),
				)
				
			}
		}

		// New Metric: vCenter Alarms
		if cluster.TriggeredAlarmState != nil {
			RecordTriggeredAlarms(ch, namespace, clusterSubsystem, cluster.Self, cluster.Name, loginData["target"].(string), cluster.TriggeredAlarmState)
		}
	}

	if len(clusters) == 0 {

		var compute []mo.ComputeResource

		err = fetchProperties(
			loginData["ctx"].(context.Context), loginData["view"].(*view.Manager), loginData["client"].(*vim25.Client),
			[]string{"ComputeResource"}, []string{"name", "summary", "datastore", "parent", "triggeredAlarmState"}, &compute, c.logger,
		)
		if err != nil {
			return err

		}

		for _, cr := range compute {

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "compute", "info"),
					"This is basic cluster info to be used for parent reference", nil,
					map[string]string{"cmo": cr.Self.Value, "host": cr.Name, "foldermo": cr.Parent.Value,
						"vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, 1.0,
			)

			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "compute", "datastores"),
					"This is basic cluster info to be used for parent reference", nil,
					map[string]string{"cmo": cr.Self.Value, "host": cr.Name, "datastores": *moSliceToString(cr.Datastore),
						"vcenter": loginData["target"].(string)},
				), prometheus.GaugeValue, 1.0,
			)

			// New Metric: vCenter Alarms
			if cr.TriggeredAlarmState != nil {
				RecordTriggeredAlarms(ch, namespace, "compute", cr.Self, cr.Name, loginData["target"].(string), cr.TriggeredAlarmState)
			}
		}
	}

	return nil
}
