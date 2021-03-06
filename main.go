package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Job struct {
	done bool
	host string
	url  *url.URL
}

type Result struct {
	done   bool
	host   string
	length int
	status int
	url    *url.URL
}

func DontRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func main() {
	path, err := os.Executable()

	if err != nil {
		panic(err)
	}

	var headers string
	var help bool
	var insecure bool
	var join bool
	var maxerrors int
	var method string
	var mindiff float64
	var nroutines int
	var statuscodes string
	var wordlist string

	dir := filepath.Dir(path)
	lists := filepath.Join(dir, "lists")
	defaultlist := filepath.Join(lists, "hosts.txt")

	flag.StringVar(&headers, "H", "", "comma-separated list/file with request headers")
	flag.StringVar(&method, "X", "GET", "request method to send (default: GET)")
	flag.Float64Var(&mindiff, "d", 0.2, "(default: 0.2)")
	flag.IntVar(&maxerrors, "e", 0, "print errors and exit after this many")
	flag.BoolVar(&help, "h", false, "show usage information and exit")
	flag.BoolVar(&join, "j", false, "send additional requests with hosts joined to URL hostnames")
	flag.BoolVar(&insecure, "k", false, "allow insecure TLS connections")
	flag.IntVar(&nroutines, "n", 40, "number of goroutines to run (default: 40)")
	flag.StringVar(&statuscodes, "s", "200", "comma-separated whitelist of status codes (default: \"200\")")
	flag.StringVar(&wordlist, "w", defaultlist, "wordlist of hosts to try")

	flag.Parse()

	if help {
		fmt.Fprintln(os.Stderr, `gh0st [OPTIONS] <file>

Options:
  -H     <headers/@file>  comma-separated list/file with request headers
  -X     <method>         request method to send (default: GET)
  -d     <float>          (default: 0.2)
  -e     <int>            print errors and exit after this many
  -h                      show usage information and exit
  -j                      send additional requests with hosts joined to URL hostnames
  -k                      allow insecure TLS connections
  -n     <int>            number of goroutines to run (default: 40)
  -s     <codes>          comma-separated whitelist of status codes (default: "200")
  -w     <file>           wordlist of hosts to try`)

		os.Exit(0)
	}

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "[!] Expected one argument <file>")
		os.Exit(1)
	}

	targetfile := flag.Arg(0)
	targetdata, err := ioutil.ReadFile(targetfile)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[!] Couldn't read <file>")
		os.Exit(1)
	}

	targets := strings.Split(string(targetdata), "\n")
	ntargets := len(targets)
	targeturls := make([]*url.URL, ntargets, ntargets)
	i := 0

	for _, target := range targets {
		if target = strings.Trim(target, " "); target == "" {
			continue
		}

		targeturl, err := url.Parse(target)

		if err != nil || !targeturl.IsAbs() {
			fmt.Fprintln(os.Stderr, "[!] Invalid URL:", target)
			os.Exit(1)
		}

		targeturls[i] = targeturl
		i++
	}

	targeturls = targeturls[:i]
	ntargets = len(targeturls)

	var headerlines []string

	if headers != "" {
		if strings.HasPrefix(headers, "@") {
			filename := string([]rune(headers)[1:])
			headerdata, err := ioutil.ReadFile(filename)

			if err != nil {
				fmt.Fprintln(os.Stderr, "[!] Can't find file with headers:", filename)
				os.Exit(1)
			}

			headerlines = strings.Split(string(headerdata), "\n")
		} else {
			headerlines = strings.Split(headers, ",")
		}
	}

	hostdata, err := ioutil.ReadFile(wordlist)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[!] Can't find wordlist:", wordlist)
		os.Exit(1)
	}

	hosts := strings.Split(string(hostdata), "\n")
	i = 0

	for _, host := range hosts {
		host = strings.Trim(host, " ")

		if host != "" {
			hosts[i] = host
			i++
		}
	}

	hosts = hosts[:i]
	nhosts := len(hosts)

	if join {
		nhosts *= 2
	}

	strcodes := strings.Split(statuscodes, ",")
	ncodes := len(strcodes)
	codes := make([]int, ncodes, ncodes)

	for i, strcode := range strcodes {
		trimcode := strings.Trim(strcode, " ")
		code, err := strconv.Atoi(trimcode)

		if err != nil || code < 100 || code > 599 {
			fmt.Fprintln(os.Stderr, "[!] Invalid status code:", trimcode)
			os.Exit(1)
		}

		codes[i] = code
	}

	banner, err := ioutil.ReadFile(filepath.Join(dir, "banner"))

	if err != nil {
		panic(err)
	}

	fmt.Fprintln(os.Stderr, string(banner))

	fmt.Fprintf(os.Stderr, "[-] Identified %d targets\n", ntargets)
	fmt.Fprintf(os.Stderr, "[-] Loaded %d Host headers\n", nhosts)
	fmt.Fprintf(os.Stderr, "[-] Total requests: %d\n", nhosts*ntargets+ntargets)
	fmt.Fprintln(os.Stderr, "[-] Status codes:", statuscodes)
	fmt.Fprintln(os.Stderr, "[-] Request method:", method)

	headermap := make(map[string]string)

	for _, line := range headerlines {
		kv := strings.SplitN(line, ":", 2)

		if len(kv) == 2 {
			key := strings.Trim(kv[0], " ")
			value := strings.Trim(kv[1], " ")
			headermap[key] = value

			fmt.Fprintf(os.Stderr, "[-] Request header > \"%s: %s\"\n", key, value)
		}
	}

	fmt.Fprintln(os.Stderr, "[-] Number of goroutines:", nroutines)
	fmt.Fprintln(os.Stderr, "[-] Minimum diff:", mindiff)

	client := &http.Client{
		CheckRedirect: DontRedirect,
		Timeout:       5 * time.Second,
	}

	jobs := make(chan *Job)
	errs := make(chan error)
	results := make(chan *Result)

	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	var wg sync.WaitGroup

	for i := 0; i < nroutines; i++ {
		go func() {
			for job := range jobs {
				if job.done {
					results <- &Result{done: true}
					return
				}

				req, err := http.NewRequest(method, job.url.String(), nil)

				if err != nil {
					errs <- err
					continue
				}

				for key, value := range headermap {
					req.Header.Add(key, value)
				}

				if job.host != "" {
					req.Host = job.host
				}

				resp, err := client.Do(req)

				if job.host == "" {
					wg.Done()
				}

				if err != nil {
					errs <- err
					continue
				}

				defer resp.Body.Close()

				data, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					errs <- err
					continue
				}

				results <- &Result{
					host:   job.host,
					length: len(data),
					status: resp.StatusCode,
					url:    job.url,
				}
			}
		}()
	}

	go func() {
		wg.Add(ntargets)

		for _, targeturl := range targeturls {
			jobs <- &Job{url: targeturl}
		}

		wg.Wait()

		fmt.Fprintln(os.Stderr, "[-] Finished reference requests")

		for _, host := range hosts {
			for _, targeturl := range targeturls {
				jobs <- &Job{
					host: host,
					url:  targeturl,
				}

				if join {
					jobs <- &Job{
						host: strings.Join([]string{host, targeturl.Hostname()}, "."),
						url:  targeturl,
					}
				}
			}
		}

		for i := 0; i < nroutines; i++ {
			jobs <- &Job{done: true}
		}

		close(jobs)
	}()

	var done = 0
	var nerrors = 0
	var size string

	all_lengths := make(map[string][]int)

outer:
	for {
		select {
		case res := <-results:
			if res.done {
				done++

				if done == nroutines {
					break outer
				}

				continue outer
			}

			if res.length == 0 {
				continue outer
			}

			for _, code := range codes {
				if code == res.status {
					host := res.url.Hostname()
					lengths, ok := all_lengths[host]

					if ok {
						for _, length := range lengths {
							diff := math.Abs(float64(length-res.length)) * 2 / float64(length+res.length)

							if diff < mindiff {
								continue outer
							}
						}
					} else {
						all_lengths[host] = []int{res.length}
						continue outer
					}

					all_lengths[host] = append(lengths, res.length)

					if res.host == "" {
						continue outer
					}

					if res.length > 1000000 {
						size = fmt.Sprintf("%.2fMB", float64(res.length)/1000000)
					} else if res.length > 1000 {
						size = fmt.Sprintf("%.2fKB", float64(res.length)/1000)
					} else {
						size = fmt.Sprintf("%dB", res.length)
					}

					fmt.Printf("%d (%s) %s - %s\n", res.status, size, res.url.String(), res.host)
					continue outer
				}
			}

		case err := <-errs:
			if maxerrors == 0 {
				continue outer
			}

			fmt.Fprintf(os.Stderr, "[!] %v\n", err)

			nerrors++

			if nerrors == maxerrors {
				fmt.Fprintln(os.Stderr, "[!] Reached max number of errors")
				fmt.Fprintln(os.Stderr, "[!] Exiting")
				os.Exit(1)
			}
		}
	}

	close(errs)
	close(results)

	fmt.Fprintln(os.Stderr, "[-] Done!")
}
