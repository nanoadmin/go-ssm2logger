# ssm2logger

A fast (like, I mean, SUPER FAST) cross-platform, headless Subaru Select Monitor 2 (SSM2) logging tool written in go.

# Why?
I built a small Raspberry Pi system to live in my 2005 Subaru Outback XT to do data logging automatically every time I drive. Most of the software out there didn't offer the solution(s) I was hoping for. They either required a UI, or were built for another embedded system with features/hardware I didn't have or care to have.

I finally found PiMonitor, which I found I could modify easily enough to run headless. Sadly, it too fell short in that is SUPER slow for datalogging (~1 sample per second).

So, I set-out to build this.

## Logging usage

### CSV logging (default)

```bash
./ssm2logger --port /dev/ttyUSB0 log --logfile-path .
```

Useful flags for `log`:

- `--defs <path>`: RomRaider logger definitions XML (default: `logger_STD_EN_v336.xml`)
- `--format <csv|ndjson>`: output mode (default: `csv`)
- `--params "name1,name2,..."`: comma-separated parameter names from the XML
- `--all`: request all ECU-supported parameters (subject to max addresses)
- `--max-addresses <int>`: cap request address count (default: `45`)
- `--logfile-path <path>`: CSV output directory (default: current directory `.`)

- `--unix-socket <path>`: send NDJSON lines to a unix domain socket instead of stdout (`--format ndjson` only)

### NDJSON logging (for MQTT pipelines)

```bash
./ssm2logger --port /dev/ttyUSB0 log \
  --defs logger_STD_EN_v370.xml \
  --format ndjson \
  --params "Engine Speed,Mass Airflow,Rear O2 Sensor,Estimated odometer"
```

Example piping NDJSON into MQTT publisher:

```bash
./ssm2logger --port /dev/ttyUSB0 log --format ndjson \
  | mosquitto_pub -l -h localhost -t subaru/telemetry
```


### NDJSON to unix socket (CSV disabled)

When you set `--format ndjson`, CSV file output is disabled.

```bash
./ssm2logger --port /dev/ttyUSB0 log \
  --format ndjson \
  --unix-socket /tmp/ssm2logger.sock
```

The logger now creates and listens on that socket path, then waits for one client to connect.

Connect with a client like:

```bash
socat -u UNIX-CONNECT:/tmp/ssm2logger.sock STDOUT
```

Start `ssm2logger` first, then connect `socat`.

### List ECU-supported parameters

```bash
./ssm2logger --port /dev/ttyUSB0 params --defs logger_STD_EN_v370.xml
```

`params` command flags:

- `--defs <path>`: RomRaider logger definitions XML
- `--format <text|ndjson>`: output format (default: `text`)

# Credits
I drew inspiration, and copied quite a lot of code from (https://github.com/src0x/LibSSM2), the .NET C# library for SSM2. In fact, I started down the path of trying to use it for my solution, but realized pretty quickly that writing cross-platform .NET Core that talks to serial ports could be quite difficult.

# TODO
* Once I get a handle on the actual SSM2 "library", break it out into it's own project, or make it easily consumable from this one
* Refactoring... There are a few things leftover from experiments.
* Support other protocols (OBD2?)
* Expand logging (like actual application logging) capabilities, with different loggers for different parts of the app, and individually assignable log levels and formatters.
* Figure out Learned Values, Switches, and reading/resetting DTCs
* Add tests
* Finish the MVP functionality
  * Consume a RomRaider XML definition file for parameters
  * Log specific PIDs to a log file
* Allow "plugins" for things that aren't SSM. Specifically my ADS_1256 DAC (https://www.waveshare.com/wiki/High-Precision_AD/DA_Board)

# Stuff I'll probably need later
* https://github.com/janne/bcm2835 - To talk to the Raspberry Pi GPIO

# Useful Stuff
* https://subdiesel.wordpress.com/2011/07/13/ssm2-via-serial-at-10400-baud/ - 10400 baud bump!
