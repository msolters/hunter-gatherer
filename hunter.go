package main

import(
  "flag"
  "io"
  "os"
  "os/exec"
  "fmt"
  "log"
  "strings"
  "strconv"
  "bufio"
  "time"
)

// Keep track of which PIDs we're already stracing
var pids_traced map[string]bool
// Any pid using over this memory percentage will be straced
var mem_mnm_threshold float64

// @brief Return a list of PIDs corresponding to all processes currently
//        exceeding the mem_threshold.
//        Ignore strace processes.
func find_high_mem_processes() ([]string) {
  var high_mem_pids []string
  mem_header := "%MEM"

  out, err := exec.Command("top", "-bn1", "-o", mem_header).Output()
  if err != nil {
    log.Fatal(err)
  }

  readings_started := false
  lines := strings.Split(string(out), "\n")
  var pid_idx, cmd_idx, mem_idx int
  for _, line := range(lines) {
    fields := strings.Fields(line)
    if len(fields) == 0 { continue }
    if readings_started {
      pid := fields[pid_idx]
      mem := fields[mem_idx]
      cmd := fields[cmd_idx]
      if _, already_traced := pids_traced[pid]; already_traced { continue }
      if cmd == "strace" || cmd == "top" { continue } // who watches the watchers?
      mem_usage, err := strconv.ParseFloat( mem, 64 )
      if err != nil { continue }
      if mem_usage < mem_mnm_threshold {
        break
      } else {
        high_mem_pids = append( high_mem_pids, pid )
        fmt.Printf("Now stracing %s (PID: %s\tMEM: %v)\n", cmd, pid, mem_usage)
      }
    } else {
      if fields[0] == "PID" && !readings_started {
        for idx, field := range(fields) {
          switch field {
          case "PID":
            pid_idx = idx
          case mem_header:
            mem_idx = idx
          case "COMMAND":
            cmd_idx = idx
          default:
            continue
          }
        }
        readings_started = true
      }
    }
  }

  return high_mem_pids
}

//  @brief  Given a std pipe and a PID, scan per line and echo with PID
//          prepended.
func trace_pipe(pid string, pipe *io.ReadCloser) {
  scanner := bufio.NewScanner(*pipe)
  scanner.Split(bufio.ScanLines)
  for scanner.Scan() {
    line := scanner.Text()
    fmt.Printf("[strace]\t%s\t%s\n", pid, line)
  }
}

//  @brief  sdfs
func trace_pid(pid string) {
  strace_cmd := exec.Command("strace", "-p", pid)
  //strace_stdout, _ := strace_cmd.StdoutPipe()
  strace_stderr, _ := strace_cmd.StderrPipe()
  strace_cmd.Start()
  //go trace_pipe(pid, &strace_stdout)  // strace only outputs to stderr
  go trace_pipe(pid, &strace_stderr)
  strace_cmd.Wait()
}

// @brief Given a list of PIDs, begin stracing each that is not already being
//        straced.
func trace_pids(pids []string) {
  for _, pid := range(pids) {
    pids_traced[pid] = true
    go trace_pid(pid)
  }
}

func main() {
  fmt.Printf("Mem Leak Hunter\n[hunter-pid]\t%v\n", os.Getpid())

  //  Parse args
  mem_threshold_str := flag.String("m", "0.0", "Minimum memory usage threshold")
  flag.Parse()

  //  Get minimum mem usage threshold
  mem_threshold, err := strconv.ParseFloat( *mem_threshold_str, 64 )
  if err != nil {
    log.Fatal(err)
    os.Exit(1)
  }
  mem_mnm_threshold = mem_threshold

  //  Init map of straced PIDs
  pids_traced = make( map[string]bool )

  // Create a ticker that outputs elapsed time
  for {
    pids := find_high_mem_processes()
    trace_pids( pids )
    time.Sleep(5 * time.Second)
  }

}
