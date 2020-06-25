# gh0st

A command-line utility for virtual host discovery

## Install

`$ go get github.com/zbo14/gh0st`

## Usage

```
             ('-. .-.            .-')    .-') _
            ( 00 )  /           ( 00 ). (  00) )
  ,----.    ,--. ,--.  .----.  (_)---\_)/     '._
 '  .-./-') |  | |  | /  ..  \ /    _ | |'--...__)
 |  |_( 00 )|   .|  |.  /  \  .\  :` `. '--.  .--'
 |  | .--, \|       ||  |  '  | '..`''.)   |  |
(|  | '. (_/|  .-.  |'  \  /  '.-._)   \   |  |
 |  '--'  | |  | |  | \  `'  / \       /   |  |
  `------'  `--' `--'  `---''   `-----'    `--'

gh0st [OPTIONS] <file>

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
  -w     <file>           wordlist of hosts to try
```
