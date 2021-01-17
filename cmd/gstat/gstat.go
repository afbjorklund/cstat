// cstat records CPU busy states. Similar to iostat, but with greater precision.
// dstat records device utilization. Similar to iostat, but with greater precision.
// mstat records Memory(+ Swap) usage. Similar to vmstat, but with more information.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var devices arrayFlags

var duration = flag.Duration("for", 365*24*time.Hour, "How long to poll until exiting")
var poll = flag.Duration("poll", 1*time.Second, "How often to poll")
var showCpu = flag.Bool("cpu", true, "show cpu")
var showDisk = flag.Bool("disk", true, "show disk")
var showMem = flag.Bool("mem", true, "show memory")
var showSwap = flag.Bool("swap", false, "show swap")
var showTotal = flag.Bool("total", false, "show total at end")

type GoStat struct {
	ct []cpu.TimesStat
	di map[string]disk.IOCountersStat
	mv *mem.VirtualMemoryStat
	ms *mem.SwapMemoryStat
}

func main() {
	flag.Var(&devices, "device", "Name of disk")
	flag.Parse()

	if len(devices) == 0 {
		partitions, err := disk.Partitions(false)
		if err != nil {
			panic(err)
		}
		for _, part := range partitions {
			// skip the loop devices
			if part.Fstype == "squashfs" {
				continue
			}
			devices = append(devices, part.Device)
		}
	}

	start := time.Now()
	lastSample := start
	stc, err := cpu.Times(false)
	if err != nil {
		panic(err)
	}
	std, err := disk.IOCounters(devices...)
	if err != nil {
		panic(err)
	}
	stm, err := mem.VirtualMemory()
	if err != nil {
		panic(err)
	}
	sts, err := mem.SwapMemory()
	if err != nil {
		panic(err)
	}
	sst := GoStat{stc, std, stm, sts}

	pst := sst
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		total(sst, pst, start, lastSample)
		os.Exit(0)
		done <- true
	}()

	for {
		if time.Since(start) > *duration {
			total(sst, pst, start, lastSample)
			os.Exit(0)
		}
		time.Sleep(*poll)

		stc, err := cpu.Times(false)
		if err != nil {
			panic(err)
		}
		std, err := disk.IOCounters(devices...)
		if err != nil {
			panic(err)
		}
		stm, err := mem.VirtualMemory()
		if err != nil {
			panic(err)
		}
		sts, err := mem.SwapMemory()
		if err != nil {
			panic(err)
		}
		st := GoStat{stc, std, stm, sts}

		lastSample = time.Now()
		display(pst, st, start, lastSample)
		pst = st
	}
}

func displayTime(start time.Time, last time.Time) {
	fmt.Printf("\"elapsed\": ")
	fmt.Printf("%d",
		int64(last.Sub(start).Milliseconds())/1000,
	)
}

func displayCpu(psta []cpu.TimesStat, sta []cpu.TimesStat, start time.Time, last time.Time) {
	pst := psta[0]
	st := sta[0]
	idle := st.Idle - pst.Idle
	total := (st.User + st.Nice + st.System + st.Idle) - (pst.User + pst.Nice + pst.System + pst.Idle)
	busy := total - idle

	fmt.Printf("\"cpu\": ")
	fmt.Printf("{ \"busy_percent\": %.2f, \"system\": %.3f, \"user\": %.3f, \"nice\": %.3f, \"idle\": %.3f }",
		float64(busy)/float64(total)*100,
		float64(st.System-pst.System)/float64(total)*100,
		float64(st.User-pst.User)/float64(total)*100,
		float64(st.Nice-pst.Nice)/float64(total)*100,
		float64(st.Idle-pst.Idle)/float64(total)*100,
	)
}


func displayDisk(psta map[string]disk.IOCountersStat, sta map[string]disk.IOCountersStat, start time.Time, last time.Time) {
	fmt.Printf("\"disk\": { ")
	first := true
	for _, device := range devices {
		if !first {
			fmt.Printf(", ")
		}
		displayN(psta, sta, start, last, device)
		first = false
	}
	fmt.Printf(" }")
}

func displayN(psta map[string]disk.IOCountersStat, sta map[string]disk.IOCountersStat, start time.Time, last time.Time, device string) {
	pst := psta[device]
	st := sta[device]

	iotime := st.IoTime - pst.IoTime
	total := last.Sub(start).Milliseconds()

	fmt.Printf("\"%s\": ",
		string(device))
	fmt.Printf("{ \"util\": %.3f, \"read\": %.3f, \"write\": %.3f }",
		float64(iotime)/float64(total)*100,
		float64(st.ReadBytes-pst.ReadBytes)/1024,
		float64(st.WriteBytes-pst.WriteBytes)/1024,
	)
}

func displayMem(st *mem.VirtualMemoryStat, start time.Time, last time.Time) {
	unit := 1024.0

	fmt.Printf("\"memory\": ")
	fmt.Printf("{ \"used_percent\": %.2f, \"total\": %.0f, \"used\": %.0f, \"free\": %.0f, \"shared\": %.0f, \"buffers\": %.0f, \"cached\": %.0f, \"available\": %.0f }",
		float64(st.UsedPercent),
		float64(st.Total)/unit,
		float64(st.Used)/unit,
		float64(st.Free)/unit,    // Linux specific
		float64(st.Shared)/unit,  // Linux specific
		float64(st.Buffers)/unit, // Linux specific
		float64(st.Cached)/unit,  // Linux specific
		float64(st.Available)/unit,
	)
}

func displaySwap(st *mem.SwapMemoryStat, start time.Time, last time.Time) {
	unit := 1024.0

	fmt.Printf("\"swap\": ")
	fmt.Printf("{ \"used_percent\": %.2f, \"total\": %.0f, \"used\": %.0f, \"free\": %.0f }",
		float64(st.UsedPercent),
		float64(st.Total)/unit,
		float64(st.Used)/unit,
		float64(st.Free)/unit,
	)
}

func display(pst GoStat, st GoStat, start time.Time, last time.Time) {
	fmt.Print("{ ")
	displayTime(start, last)
	if *showCpu {
		fmt.Print(", ")
		displayCpu(pst.ct, st.ct, start, last)
	}
	if *showDisk {
		fmt.Print(", ")
		displayDisk(pst.di, st.di, start, last)
	}
	if *showMem {
		fmt.Print(", ")
		displayMem(st.mv, start, last)
	}
	if *showSwap {
		fmt.Print(", ")
		displaySwap(st.ms, start, last)
	}
	fmt.Println(" }")
}

func total(pst GoStat, st GoStat, start time.Time, last time.Time) {
	if *showTotal {
		fmt.Printf("\n\n// measured average over %s\n", last.Sub(start))
		display(pst, st, start, last)
	}
}
