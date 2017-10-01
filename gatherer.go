package main

import(
  "flag"
  "os"
  "os/exec"
  "os/signal"
  "fmt"
  "log"
  "bufio"
  "strings"
)

// Keep track of which PIDs we're already stracing
var pod, container, hunter_pid string
type PIDWriter struct {
  file *os.File
  writer *bufio.Writer
}
var pids_traced map[string]PIDWriter

//  @brief  Compile the hunter.go program.
//          Copy it to the remote pod/container target.
func install() {
  fmt.Printf("Building remote script...")
  _, build_err := exec.Command("bash", "build-hunter.sh").Output()
  if build_err != nil {
    log.Fatal(build_err)
    os.Exit(1)
  }
  fmt.Println("done.")

  fmt.Printf("Installing remote script...")
  destination_script := fmt.Sprintf("%s:/tmp/hunter", pod)
  _, copy_err := exec.Command("kubectl", "cp", "hunter", destination_script, "-c", container).Output()
  if copy_err != nil {
    log.Fatal(copy_err)
    os.Exit(1)
  }
  fmt.Println("done.")
}

//  @brief  Create a local log file and buffered writer object to handle
//          streaming strace data to that file.
func create_pid_writer(pid string) {
  file_name := fmt.Sprintf("%s-strace.log", pid)
  f, err := os.Create(file_name)
  if err != nil {
    log.Fatal(err)
    return
  }
  w := bufio.NewWriter(f)
  pids_traced[pid] = PIDWriter{f, w}
}

//  @brief  Flush all buffer writers and close all log files.
func clean_up_files() {
  for _, pid_writer := range pids_traced {
    pid_writer.writer.Flush()
    pid_writer.file.Close()
  }
}

//  @brief
func parse_line(line string) {
  //fmt.Println(line)
  switch true {
  case strings.Contains(line, "[hunter-pid]"):
    fields := strings.Fields(line)
    hunter_pid = fields[1]
  case strings.Contains(line, "[strace]"):
    fields := strings.Fields(line)
    pid := fields[1]
    data_index := len(fields[0]) + len(pid) + 2*len("\t")
    if _, already_traced := pids_traced[pid]; !already_traced {
      create_pid_writer(pid)
    }
    data := fmt.Sprintf("%s\n", line[data_index:])
    fmt.Printf("[%s]: %s", pid, data)
    pids_traced[pid].writer.WriteString( data )
  default:
    fmt.Println(line)
  }
}

//  @brief
func terminate_hunter() {
  fmt.Printf("Terminating remote script...\n")
  _, kill_err := exec.Command("kubectl", "exec", pod, "-c", container, "--", "kill", "-15", hunter_pid).Output()
  if kill_err != nil {
    log.Fatal(kill_err)
    fmt.Printf("Error terminating remote script!  PID %s may now be a zombie process.\n", hunter_pid)
    os.Exit(1)
  }
  fmt.Println("done.")
  os.Exit(0)
}

//  @brief  kubectl exec the hunter program on the remote container.
//          Listen to and parse the results.
func listen_to_remote(mem_threshold string) {
  fmt.Printf("\nstracing processes with %%MEM > %s%%:\n", mem_threshold)
  fmt.Printf("Connecting to %s/%s:\n\n", pod, container)
  listen_cmd := exec.Command("kubectl", "exec", pod, "-c", container, "-t", "--", "/tmp/hunter", "-m", mem_threshold)
  listen_stdout, _ := listen_cmd.StdoutPipe()
  listen_cmd.Start()

  scanner := bufio.NewScanner(listen_stdout)
  scanner.Split(bufio.ScanLines)
  for scanner.Scan() {
    m := scanner.Text()
    parse_line( m )
  }

  listen_cmd.Wait()
}

func main() {
  //  Parse arguments
  pod_arg := flag.String("p", "", "Name of the problem pod")
  container_arg := flag.String("c", "", "Name of the container in peril")
  mem_threshold_arg := flag.String("m", "0.0", "Minimum memory usage (interpreted as percentage, like 33.3) to trigger a process strace")
  installation_arg := flag.Bool("i", false, "Install remote script to target container?")
  flag.Parse()

  pod = *pod_arg
  container = *container_arg
  if len(pod) == 0 {
    fmt.Println("Must specify a target pod with -p!")
    os.Exit(1)
  }
  if len(container) == 0 {
    fmt.Println("Must specify a target container with -c!")
    os.Exit(1)
  }

  if *installation_arg { install() }

  //  Catch kill signals to shutdown gracefully
  go func(){
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    <-c
    clean_up_files()
    terminate_hunter()
  }()

  pids_traced = make( map[string]PIDWriter )
  //  Connect to the target container!
  listen_to_remote( *mem_threshold_arg )

}
