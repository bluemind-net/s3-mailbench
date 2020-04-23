package main

import (
	"encoding/csv"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"io"
	"sort"
	"time"
)

const (
	min int = iota + 1
	max
	avg
	p25
	p50
	p75
	p90
	p99
)

type Stats struct {
	Title string
	// Latency in milliseconds averages
	Latency    map[int]float64
	DataPoints []Result
	SumLatency uint64
	SumBytes   uint64
	Start      time.Time
	TotalTime  time.Duration
	Rate       float64
	Count      uint64
	ObjectRate float64
}

func NewStats(title string) *Stats {
	return &Stats{
		Title:      title,
		Latency:    make(map[int]float64),
		DataPoints: make([]Result, 0),
		SumLatency: 0,
		SumBytes:   0,
		Count:      0,
		Start:      time.Now(),
		Rate:       0,
		ObjectRate:	0,
	}
}

func PrintStats(writer io.Writer, stats []*Stats) {
	table := tablewriter.NewWriter(writer)
	table.SetHeader(GetHeader())
	for _, stat := range stats {
		stat.Refresh()
		table.Append(stat.GetData())
	}
	table.Render()
}

func WriteCSV(writer io.Writer, stats[] *Stats) {
	w := csv.NewWriter(writer)
	w.Write(GetHeader())
	for _, stat := range stats {
		stat.Refresh()
		w.Write(stat.GetData())
	}
	w.Flush()
}

func (stats *Stats) Update(result Result) {
	stats.SumBytes += uint64(result.Size)
	stats.SumLatency += uint64(result.Latency.Nanoseconds())
	stats.DataPoints = append(stats.DataPoints, result)
	stats.Count++
	totalTime := time.Now().Sub(stats.Start)
	stats.Rate = (float64(stats.SumBytes)) / (totalTime.Seconds())
	stats.ObjectRate = (float64(stats.Count)) / (totalTime.Seconds())
}

func (stats *Stats) Refresh() {
	if stats.Count <= 0 {
		return
	}
	// calculate the summary statistics for the last byte latencies
	sort.Sort(ByLatency(stats.DataPoints))
	stats.Latency[avg] = (float64(stats.SumLatency) / float64(stats.Count)) / 1_000_000
	stats.Latency[min] = float64(stats.DataPoints[0].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[max] = float64(stats.DataPoints[len(stats.DataPoints)-1].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[p25] = float64(stats.DataPoints[int(float64(stats.Count)*float64(0.25))-1].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[p50] = float64(stats.DataPoints[int(float64(stats.Count)*float64(0.5))-1].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[p75] = float64(stats.DataPoints[int(float64(stats.Count)*float64(0.75))-1].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[p90] = float64(stats.DataPoints[int(float64(stats.Count)*float64(0.90))-1].Latency.Nanoseconds()) / 1_000_000
	stats.Latency[p99] = float64(stats.DataPoints[int(float64(stats.Count)*float64(0.99))-1].Latency.Nanoseconds()) / 1_000_000
}

func GetHeader() []string {
	return []string{
		"Test", "Throughput", "Rate",
		"avg", "p25", "p50", "p75", "p90", "p99", "max",
	}
}

func (stats *Stats) GetData() []string {
	return []string{
		stats.Title,
		fmt.Sprintf("%s/s", byteFormat(stats.Rate)),
		fmt.Sprintf("%.0f obj/s", stats.ObjectRate),
		fmt.Sprintf("%.0f", stats.Latency[avg]),
		fmt.Sprintf("%.0f", stats.Latency[p25]),
		fmt.Sprintf("%.0f", stats.Latency[p50]),
		fmt.Sprintf("%.0f", stats.Latency[p75]),
		fmt.Sprintf("%.0f", stats.Latency[p90]),
		fmt.Sprintf("%.0f", stats.Latency[p99]),
		fmt.Sprintf("%.0f", stats.Latency[max]),
	}
}

func (stats *Stats) Print(writer io.Writer) {
	stats.Refresh()
	table := tablewriter.NewWriter(writer)
	table.SetHeader(GetHeader())
	table.Append(stats.GetData())
	table.Render()
}

// formats bytes to KB or MB
func byteFormat(bytes float64) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.2f MiB", bytes/1024/1024)
	}
	return fmt.Sprintf("%.2f KiB", bytes/1024)
}

// comparator to sort by last byte latency
type ByLatency []Result

func (a ByLatency) Len() int           { return len(a) }
func (a ByLatency) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByLatency) Less(i, j int) bool { return a[i].Latency < a[j].Latency }
