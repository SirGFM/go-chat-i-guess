package main

import (
    "encoding/json"
    "flag"
    "log"
    "os"
)

type Args struct {
    // IP on which the server will accept connections. Defaults to 0.0.0.0
    IP string
    // Port on which the server will accept connections. Defaults to 8888
    Port int
    // ReadSize allocated for gorilla-ws's buffer when a new connection is accepted. Defaults to 1024
    ReadSize int
    // WriteSize allocated for gorilla-ws's buffer when a new connection is accepted. Defaults to 1024
    WriteSize int
    // IgnoreOrigin and accept connections from any source (mostly for development)
    IgnoreOrigin bool
}

// parseArgs either from the command line or from the supplied JSON file.
//
// If a JSON file is supplied, it's used as the default parameters, which may be overriden by CLI-supplied arguments.
func parseArgs() Args {
    var args Args
    var confFile string
    const defaultIP = "0.0.0.0"
    const defaultPort = 8888
    const defaultReadSize = 1024
    const defaultWriteSize = 1024
    const defaultIgnoreOrigin = true

    flag.StringVar(&args.IP, "IP", defaultIP, "IP on which the server will accept connections")
    flag.IntVar(&args.Port, "Port", defaultPort, "Port on which the server will accept connections")
    flag.IntVar(&args.ReadSize, "ReadSize", defaultReadSize, "ReadSize allocated for gorilla-ws's buffer when a new connection is accepted")
    flag.IntVar(&args.WriteSize, "WriteSize", defaultWriteSize, "WriteSize allocated for gorilla-ws's buffer when a new connection is accepted")
    flag.BoolVar(&args.IgnoreOrigin, "IgnoreOrigin", defaultIgnoreOrigin, "IgnoreOrigin and accept connections from any source (mostly for development)")
    flag.StringVar(&confFile, "confFile", "", "JSON file with the configuration options. May be overriden by other CLI arguments")
    flag.Parse()

    if len(confFile) != 0 {
        var jsonArgs Args

        f, err := os.Open(confFile)
        if err != nil {
            log.Fatalf("Couldn't open the configuration file '%+v': %+v", confFile, err)
        }
        defer f.Close()

        dec := json.NewDecoder(f)
        err = dec.Decode(&jsonArgs)
        if err != nil {
            log.Fatalf("Couldn't decode the configuration file '%+v': %+v", confFile, err)
        }

        // Walk over every set argument to override the JSON file
        flag.Visit(func (f *flag.Flag) {
            if f.Name == "confFile" {
                // Skip the file itself
                return
            }

            var tmp interface{}
            tmp = f.Value
            get, ok := tmp.(flag.Getter)
            if !ok {
                log.Fatalf("'%s' doesn't have an associated flag.Getter", f.Name)
            }

            switch f.Name {
            case "IP":
                val, _ := get.Get().(string)
                log.Printf("Overriding JSON's IP (%+v) with CLI's value (%+v)", jsonArgs.IP, val)
                jsonArgs.IP = val
            case "Port":
                val, _ := get.Get().(int)
                log.Printf("Overriding JSON's Port (%+v) with CLI's value (%+v)", jsonArgs.Port, val)
                jsonArgs.Port = val
            case "ReadSize":
                val, _ := get.Get().(int)
                log.Printf("Overriding JSON's ReadSize (%+v) with CLI's value (%+v)", jsonArgs.ReadSize, val)
                jsonArgs.ReadSize = val
            case "WriteSize":
                val, _ := get.Get().(int)
                log.Printf("Overriding JSON's WriteSize (%+v) with CLI's value (%+v)", jsonArgs.WriteSize, val)
                jsonArgs.WriteSize = val
            case "IgnoreOrigin":
                val, _ := get.Get().(bool)
                log.Printf("Overriding JSON's IgnoreOrigin (%+v) with CLI's value (%+v)", jsonArgs.IgnoreOrigin, val)
                jsonArgs.IgnoreOrigin = val
            }
        })

        args = jsonArgs
    }

    log.Printf("Starting server with options:")
    log.Printf("  - IP: %+v", args.IP)
    log.Printf("  - Port: %+v", args.Port)
    log.Printf("  - ReadSize: %+v", args.ReadSize)
    log.Printf("  - WriteSize: %+v", args.WriteSize)
    log.Printf("  - IgnoreOrigin: %+v", args.IgnoreOrigin)

    return args
}
