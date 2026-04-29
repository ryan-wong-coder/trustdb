package observability

import (
	"fmt"
	"time"

	pdb "github.com/cockroachdb/pebble"
	"github.com/prometheus/client_golang/prometheus"
)

type pebbleMetricsProvider interface {
	PebbleMetrics() *pdb.Metrics
}

type pebbleCollector struct {
	source pebbleMetricsProvider

	compactionsTotal               *prometheus.Desc
	compactionDurationSecondsTotal *prometheus.Desc
	compactionDebtBytes            *prometheus.Desc
	compactionInProgressBytes      *prometheus.Desc
	compactionInProgressTotal      *prometheus.Desc
	flushesTotal                   *prometheus.Desc
	flushInProgressTotal           *prometheus.Desc
	ingestionsTotal                *prometheus.Desc
	memtableSizeBytes              *prometheus.Desc
	memtableCount                  *prometheus.Desc
	memtableZombieSizeBytes        *prometheus.Desc
	readAmplification              *prometheus.Desc
	diskSpaceUsageBytes            *prometheus.Desc
	snapshotsOpen                  *prometheus.Desc
	tableObsoleteSizeBytes         *prometheus.Desc
	tableZombieSizeBytes           *prometheus.Desc
	backingTableSizeBytes          *prometheus.Desc
	backingTables                  *prometheus.Desc
	walLiveFiles                   *prometheus.Desc
	walObsoleteFiles               *prometheus.Desc
	walSizeBytes                   *prometheus.Desc
	walPhysicalSizeBytes           *prometheus.Desc
	walBytesInTotal                *prometheus.Desc
	walBytesWrittenTotal           *prometheus.Desc
	levelSizeBytes                 *prometheus.Desc
	levelFiles                     *prometheus.Desc
	levelScore                     *prometheus.Desc
}

// RegisterPebbleMetrics installs a custom collector when the proofstore
// is backed by Pebble. Non-Pebble stores are ignored so callers can wire
// this unconditionally.
func RegisterPebbleMetrics(reg prometheus.Registerer, store any) (bool, error) {
	source, ok := store.(pebbleMetricsProvider)
	if !ok || source == nil {
		return false, nil
	}
	if err := reg.Register(NewPebbleCollector(source)); err != nil {
		return false, err
	}
	return true, nil
}

// NewPebbleCollector exports a bounded, low-cardinality subset of the
// Pebble engine metrics so operators can correlate ingest throughput with
// memtable pressure, WAL growth, and compaction debt.
func NewPebbleCollector(source pebbleMetricsProvider) prometheus.Collector {
	return &pebbleCollector{
		source: source,
		compactionsTotal: prometheus.NewDesc(
			"trustdb_pebble_compactions_total",
			"Total number of Pebble compactions since the database was opened.",
			nil, nil,
		),
		compactionDurationSecondsTotal: prometheus.NewDesc(
			"trustdb_pebble_compaction_duration_seconds_total",
			"Cumulative duration of Pebble compactions since the database was opened.",
			nil, nil,
		),
		compactionDebtBytes: prometheus.NewDesc(
			"trustdb_pebble_compaction_debt_bytes",
			"Estimated Pebble compaction debt in bytes.",
			nil, nil,
		),
		compactionInProgressBytes: prometheus.NewDesc(
			"trustdb_pebble_compaction_in_progress_bytes",
			"Bytes currently being written by in-progress Pebble compactions.",
			nil, nil,
		),
		compactionInProgressTotal: prometheus.NewDesc(
			"trustdb_pebble_compaction_in_progress_total",
			"Current number of in-progress Pebble compactions.",
			nil, nil,
		),
		flushesTotal: prometheus.NewDesc(
			"trustdb_pebble_flushes_total",
			"Total number of Pebble flushes since the database was opened.",
			nil, nil,
		),
		flushInProgressTotal: prometheus.NewDesc(
			"trustdb_pebble_flush_in_progress_total",
			"Current number of in-progress Pebble flushes.",
			nil, nil,
		),
		ingestionsTotal: prometheus.NewDesc(
			"trustdb_pebble_ingestions_total",
			"Total number of Pebble ingestions since the database was opened.",
			nil, nil,
		),
		memtableSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_memtable_size_bytes",
			"Bytes allocated by Pebble memtables and flushable batches.",
			nil, nil,
		),
		memtableCount: prometheus.NewDesc(
			"trustdb_pebble_memtable_count",
			"Current number of Pebble memtables.",
			nil, nil,
		),
		memtableZombieSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_memtable_zombie_size_bytes",
			"Bytes retained by zombie Pebble memtables.",
			nil, nil,
		),
		readAmplification: prometheus.NewDesc(
			"trustdb_pebble_read_amplification",
			"Approximate Pebble read amplification across the LSM tree.",
			nil, nil,
		),
		diskSpaceUsageBytes: prometheus.NewDesc(
			"trustdb_pebble_disk_space_usage_bytes",
			"Approximate on-disk space used by the Pebble database.",
			nil, nil,
		),
		snapshotsOpen: prometheus.NewDesc(
			"trustdb_pebble_snapshots_open",
			"Current number of open Pebble snapshots.",
			nil, nil,
		),
		tableObsoleteSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_table_obsolete_size_bytes",
			"Bytes held by obsolete Pebble tables waiting for cleanup.",
			nil, nil,
		),
		tableZombieSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_table_zombie_size_bytes",
			"Bytes held by zombie Pebble tables still referenced by iterators.",
			nil, nil,
		),
		backingTableSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_backing_table_size_bytes",
			"Total bytes of Pebble backing SSTables.",
			nil, nil,
		),
		backingTables: prometheus.NewDesc(
			"trustdb_pebble_backing_tables",
			"Current number of Pebble backing SSTables.",
			nil, nil,
		),
		walLiveFiles: prometheus.NewDesc(
			"trustdb_pebble_wal_live_files",
			"Current number of live Pebble WAL files.",
			nil, nil,
		),
		walObsoleteFiles: prometheus.NewDesc(
			"trustdb_pebble_wal_obsolete_files",
			"Current number of obsolete Pebble WAL files.",
			nil, nil,
		),
		walSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_wal_size_bytes",
			"Bytes of live data currently present in Pebble WAL files.",
			nil, nil,
		),
		walPhysicalSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_wal_physical_size_bytes",
			"Physical on-disk bytes currently used by Pebble WAL files.",
			nil, nil,
		),
		walBytesInTotal: prometheus.NewDesc(
			"trustdb_pebble_wal_bytes_in_total",
			"Logical bytes written into the Pebble WAL since the database was opened.",
			nil, nil,
		),
		walBytesWrittenTotal: prometheus.NewDesc(
			"trustdb_pebble_wal_bytes_written_total",
			"Physical bytes written to the Pebble WAL since the database was opened.",
			nil, nil,
		),
		levelSizeBytes: prometheus.NewDesc(
			"trustdb_pebble_level_size_bytes",
			"Current bytes stored in each Pebble LSM level.",
			[]string{"level"}, nil,
		),
		levelFiles: prometheus.NewDesc(
			"trustdb_pebble_level_files",
			"Current number of files in each Pebble LSM level.",
			[]string{"level"}, nil,
		),
		levelScore: prometheus.NewDesc(
			"trustdb_pebble_level_score",
			"Current compaction score of each Pebble LSM level.",
			[]string{"level"}, nil,
		),
	}
}

func (c *pebbleCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{
		c.compactionsTotal,
		c.compactionDurationSecondsTotal,
		c.compactionDebtBytes,
		c.compactionInProgressBytes,
		c.compactionInProgressTotal,
		c.flushesTotal,
		c.flushInProgressTotal,
		c.ingestionsTotal,
		c.memtableSizeBytes,
		c.memtableCount,
		c.memtableZombieSizeBytes,
		c.readAmplification,
		c.diskSpaceUsageBytes,
		c.snapshotsOpen,
		c.tableObsoleteSizeBytes,
		c.tableZombieSizeBytes,
		c.backingTableSizeBytes,
		c.backingTables,
		c.walLiveFiles,
		c.walObsoleteFiles,
		c.walSizeBytes,
		c.walPhysicalSizeBytes,
		c.walBytesInTotal,
		c.walBytesWrittenTotal,
		c.levelSizeBytes,
		c.levelFiles,
		c.levelScore,
	} {
		ch <- desc
	}
}

func (c *pebbleCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.source == nil {
		return
	}
	metrics := c.source.PebbleMetrics()
	if metrics == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.compactionsTotal, prometheus.CounterValue, float64(metrics.Compact.Count))
	ch <- prometheus.MustNewConstMetric(c.compactionDurationSecondsTotal, prometheus.CounterValue, durationSeconds(metrics.Compact.Duration))
	ch <- prometheus.MustNewConstMetric(c.compactionDebtBytes, prometheus.GaugeValue, float64(metrics.Compact.EstimatedDebt))
	ch <- prometheus.MustNewConstMetric(c.compactionInProgressBytes, prometheus.GaugeValue, float64(metrics.Compact.InProgressBytes))
	ch <- prometheus.MustNewConstMetric(c.compactionInProgressTotal, prometheus.GaugeValue, float64(metrics.Compact.NumInProgress))
	ch <- prometheus.MustNewConstMetric(c.flushesTotal, prometheus.CounterValue, float64(metrics.Flush.Count))
	ch <- prometheus.MustNewConstMetric(c.flushInProgressTotal, prometheus.GaugeValue, float64(metrics.Flush.NumInProgress))
	ch <- prometheus.MustNewConstMetric(c.ingestionsTotal, prometheus.CounterValue, float64(metrics.Ingest.Count))
	ch <- prometheus.MustNewConstMetric(c.memtableSizeBytes, prometheus.GaugeValue, float64(metrics.MemTable.Size))
	ch <- prometheus.MustNewConstMetric(c.memtableCount, prometheus.GaugeValue, float64(metrics.MemTable.Count))
	ch <- prometheus.MustNewConstMetric(c.memtableZombieSizeBytes, prometheus.GaugeValue, float64(metrics.MemTable.ZombieSize))
	ch <- prometheus.MustNewConstMetric(c.readAmplification, prometheus.GaugeValue, float64(metrics.ReadAmp()))
	ch <- prometheus.MustNewConstMetric(c.diskSpaceUsageBytes, prometheus.GaugeValue, float64(metrics.DiskSpaceUsage()))
	ch <- prometheus.MustNewConstMetric(c.snapshotsOpen, prometheus.GaugeValue, float64(metrics.Snapshots.Count))
	ch <- prometheus.MustNewConstMetric(c.tableObsoleteSizeBytes, prometheus.GaugeValue, float64(metrics.Table.ObsoleteSize))
	ch <- prometheus.MustNewConstMetric(c.tableZombieSizeBytes, prometheus.GaugeValue, float64(metrics.Table.ZombieSize))
	ch <- prometheus.MustNewConstMetric(c.backingTableSizeBytes, prometheus.GaugeValue, float64(metrics.Table.BackingTableSize))
	ch <- prometheus.MustNewConstMetric(c.backingTables, prometheus.GaugeValue, float64(metrics.Table.BackingTableCount))
	ch <- prometheus.MustNewConstMetric(c.walLiveFiles, prometheus.GaugeValue, float64(metrics.WAL.Files))
	ch <- prometheus.MustNewConstMetric(c.walObsoleteFiles, prometheus.GaugeValue, float64(metrics.WAL.ObsoleteFiles))
	ch <- prometheus.MustNewConstMetric(c.walSizeBytes, prometheus.GaugeValue, float64(metrics.WAL.Size))
	ch <- prometheus.MustNewConstMetric(c.walPhysicalSizeBytes, prometheus.GaugeValue, float64(metrics.WAL.PhysicalSize))
	ch <- prometheus.MustNewConstMetric(c.walBytesInTotal, prometheus.CounterValue, float64(metrics.WAL.BytesIn))
	ch <- prometheus.MustNewConstMetric(c.walBytesWrittenTotal, prometheus.CounterValue, float64(metrics.WAL.BytesWritten))
	for level, lm := range metrics.Levels {
		if lm.NumFiles == 0 && lm.Size == 0 && lm.Score == 0 && lm.Sublevels == 0 {
			continue
		}
		label := fmt.Sprintf("L%d", level)
		ch <- prometheus.MustNewConstMetric(c.levelSizeBytes, prometheus.GaugeValue, float64(lm.Size), label)
		ch <- prometheus.MustNewConstMetric(c.levelFiles, prometheus.GaugeValue, float64(lm.NumFiles), label)
		ch <- prometheus.MustNewConstMetric(c.levelScore, prometheus.GaugeValue, lm.Score, label)
	}
}

func durationSeconds(d time.Duration) float64 {
	return float64(d) / float64(time.Second)
}
