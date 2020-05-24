# gh0st

A command line utility for virtual host discovery

## Install

`go get github.com/zbo14/gh0st`

## Usage

```
gh0st [OPTIONS] <url>

Options:
  -H   <headers/@file>  comma-separated list/file with request headers
  -X   <method>         request method to send (default: GET)
  -e   <int>            exit after this many errors (default: 3)
  -h                    show usage information and exit
  -hl  <file>           file with list of hosts
  -n   <int>            number of goroutines to run (default: 40)
  -s   <codes>          comma-separated whitelist of status codes (default: "200,204,301,302,307,401,403")
```
