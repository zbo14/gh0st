package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Result struct {
	done   bool
	host   string
	length int
	status int
}

func main() {
	path, err := os.Executable()

	if err != nil {
		panic(err)
	}

	var headers string
	var help bool
	var hostlist string
	var maxerrors int
	var method string
	var nroutines int
	var statuscodes string

	dir := filepath.Dir(path)
	lists := filepath.Join(dir, "lists")
	hlist := filepath.Join(lists, "hosts.txt")

	flag.StringVar(&headers, "H", "", "comma-separated list/file with request headers")
	flag.StringVar(&method, "X", "GET", "request method to send")
	flag.IntVar(&maxerrors, "e", 3, "exit after this many errors")
	flag.BoolVar(&help, "h", false, "show usage information and exit")
	flag.StringVar(&hostlist, "hl", hlist, "file with list of hosts")
	flag.IntVar(&nroutines, "n", 40, "number of goroutines to run")
	flag.StringVar(&statuscodes, "s", "200,204,301,302,307,401,403", "comma-separated whitelist of status codes")

	flag.Parse()

	if help {
		fmt.Fprintln(os.Stderr, `gh0st [OPTIONS] <url>

Options:
  -H   <headers/@file>  comma-separated list/file with request headers
  -X   <method>         request method to send (default: GET)
  -e   <int>            exit after this many errors (default: 3)
  -h                    show usage information and exit
  -hl  <file>           file with list of hosts
  -n   <int>            number of goroutines to run (default: 40)
  -s   <codes>          comma-separated whitelist of status codes (default: "200,204,301,302,307,401,403")`)

		os.Exit(0)
	}

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "[!] Expected one argument <url>")
		os.Exit(1)
	}

	urlstr := flag.Arg(0)
	url, err := url.Parse(urlstr)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[!] Invalid URL:", urlstr)
		os.Exit(1)
	}

	if !url.IsAbs() {
		fmt.Fprintln(os.Stderr, "[!] Expected absolute URL")
		os.Exit(1)
	}

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

	hostdata, err := ioutil.ReadFile(hostlist)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[!] Can't find hostlist:", hostlist)
		os.Exit(1)
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

	hostname := url.Hostname()

	fmt.Fprintln(os.Stderr, "[-] Original host:", hostname)
	fmt.Fprintln(os.Stderr, "[-] Method:", method)

	headermap := make(map[string]string)

	for _, line := range headerlines {
		kv := strings.SplitN(line, ":", 2)

		if len(kv) == 2 {
			key := strings.Trim(kv[0], " ")
			value := strings.Trim(kv[1], " ")
			headermap[key] = value

			fmt.Fprintf(os.Stderr, "[-] Header > \"%s: %s\"\n", key, value)
		}
	}

	hostlines := strings.Split(string(hostdata), "\n")
	nhosts := len(hostlines) * 2

	if nroutines > nhosts {
		if nhosts < 40 {
			nroutines = nhosts
		} else {
			nroutines = 40
		}

		fmt.Fprintf(os.Stderr, "[-] Reducing number of goroutines to %d\n", nroutines)
	}

	fmt.Fprintf(os.Stderr, "[-] Sending %d requests\n", nhosts)

	client := &http.Client{}
	hosts := make(chan string)
	errs := make(chan error)
	results := make(chan *Result)

	for i := 0; i < nroutines; i++ {
		go func() {
			var host string
			var length int

			for host = range hosts {
				if host == "DONE" {
					results <- &Result{done: true}
					return
				}

				req, err := http.NewRequest(method, url.String(), nil)

				if err != nil {
					errs <- err
					continue
				}

				for key, value := range headermap {
					req.Header.Add(key, value)
				}

				req.Host = host
				resp, err := client.Do(req)

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

				length = len(data)

				results <- &Result{
					host:   host,
					length: length,
					status: resp.StatusCode,
				}
			}
		}()
	}

	go func() {
		for _, line := range hostlines {
			host := strings.Trim(line, " \n")

			if host == "" {
				continue
			}

			hosts <- host
			hosts <- strings.Join([]string{host, hostname}, ".")
		}

		for i := 0; i < nroutines; i++ {
			hosts <- "DONE"
		}

		close(hosts)
	}()

	var done = 0
	var nerrors = 0
	var size string

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

				for _, code := range codes {
					if code == res.status {
						if res.length > 1000 {
							size = fmt.Sprintf("%.1fKB", float64(res.length) / 1000)
						}	else {
							size = fmt.Sprintf("%dB", res.length)
						}

						fmt.Printf("[+] %d (%s) - %s\n", res.status, size, res.host)
						continue outer
					}
				}

			case err := <-errs:
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
