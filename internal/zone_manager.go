package internal

import (
	"fmt"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"
)

type ZoneManager struct {
	zones []string
}

func NewZoneManager(zones []string) *ZoneManager {
	if len(zones) == 0 {
		zones = []string{
			"region-name.az01arm",
			"region-name.az02arm",
			"region-name.az03arm",
		}
	}
	return &ZoneManager{zones: zones}
}

func (zm *ZoneManager) GetZones() []string {
	return zm.zones
}

type ZoneStats struct {
	Zone           string
	PrimaryCount   int
	BackupCount    int
	FailedCount    int
	PromotedCount  int
	OverloadedCount int
	TotalNodes     int
}

func (zm *ZoneManager) CalculateZoneStats(nodes []NodeInfo, promotedNodes map[string]string) map[string]ZoneStats {
	stats := make(map[string]ZoneStats)

	for _, zone := range zm.zones {
		stats[zone] = ZoneStats{Zone: zone}
	}

	for _, node := range nodes {
		zone := node.Zone
		if zone == "" {
			continue
		}

		if s, ok := stats[zone]; ok {
			s.TotalNodes++
			if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
				s.PrimaryCount++
			}
			if node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
				node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" {
				s.BackupCount++
			}
			if node.Labels[nodeoperatorv1alpha1.LabelFailed] == "true" {
				s.FailedCount++
			}
			if _, promoted := promotedNodes[node.Name]; promoted {
				s.PromotedCount++
			}
			for _, taint := range node.Taints {
				if taint.Key == nodeoperatorv1alpha1.TaintOverloaded {
					s.OverloadedCount++
					break
				}
			}
			stats[zone] = s
		}
	}

	return stats
}

func (zm *ZoneManager) GetZoneForNode(node NodeInfo) string {
	return node.Zone
}

func (zm *ZoneManager) NeedsPromotion(stats map[string]ZoneStats, zone string, requiredPrimaries int) bool {
	if s, ok := stats[zone]; ok {
		availablePrimaries := s.PrimaryCount - s.OverloadedCount
		return availablePrimaries < requiredPrimaries
	}
	return true
}

func (zm *ZoneManager) GetZoneWithMostBackups(stats map[string]ZoneStats) string {
	var maxBackupCount int
	var zoneWithMostBackups string

	for zone, s := range stats {
		if s.BackupCount > maxBackupCount {
			maxBackupCount = s.BackupCount
			zoneWithMostBackups = zone
		}
	}

	return zoneWithMostBackups
}

func (zm *ZoneManager) GetZonesNeedingPromotion(stats map[string]ZoneStats, requiredPrimaries int) []string {
	var zones []string
	for zone, s := range stats {
		availablePrimaries := s.PrimaryCount - s.OverloadedCount
		if availablePrimaries < requiredPrimaries {
			zones = append(zones, zone)
		}
	}
	return zones
}

func (zm *ZoneManager) CanDemote(stats map[string]ZoneStats, zone string, initialBackupCount map[string]int) bool {
	if s, ok := stats[zone]; ok {
		expectedBackupCount := initialBackupCount[zone]
		currentBackupCount := s.BackupCount + s.PromotedCount
		return currentBackupCount > expectedBackupCount
	}
	return false
}

func (zm *ZoneManager) GetZoneForDemotion(stats map[string]ZoneStats, zone string) string {
	if s, ok := stats[zone]; ok && s.PromotedCount > 0 {
		return zone
	}
	return ""
}

func (zm *ZoneManager) PrintZoneStats(stats map[string]ZoneStats) string {
	str := "Zone Statistics:\n"
	for zone, s := range stats {
		str += fmt.Sprintf("  Zone: %s\n", zone)
		str += fmt.Sprintf("    Primary: %d, Backup: %d, Failed: %d, Promoted: %d, Overloaded: %d, Total: %d\n",
			s.PrimaryCount, s.BackupCount, s.FailedCount, s.PromotedCount, s.OverloadedCount, s.TotalNodes)
	}
	return str
}
