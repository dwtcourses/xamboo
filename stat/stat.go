package stat

import (
	"net"
	"sync"
	"time"

	"github.com/webability-go/xamboo/assets"
	"github.com/webability-go/xamboo/config"
	"github.com/webability-go/xamboo/logger"
)

/*
This code keeps tracks and stats of the whole webserver and served pages and requests
*/

type RequestStat struct {
	Id        uint64
	StartTime time.Time
	Time      time.Time
	Hostname  string
	Request   string
	Protocol  string
	Method    string
	Code      int
	Length    int
	Duration  time.Duration
	IP        string
	Port      string
	Alive     bool
	Context   *assets.Context `json:"-"`
}

type SiteStat struct {
	RequestsTotal  int            // num requests total, anything included
	RequestsServed map[int]int    // by response code
	LengthServed   int            // length total, anything included
	Requests       []*RequestStat // the last minute requests
}

type Stat struct {
	Start          time.Time
	RequestsTotal  int            // num requests total, anything included
	LengthServed   int            // length total, anything included
	RequestsServed map[int]int    // by response code
	Requests       []*RequestStat // by microtime. keep last minute requests

	SitesStat map[string]*SiteStat // Every site stat. referenced by ID (from config)

	mutex sync.RWMutex
}

var SystemStat *Stat
var RequestCounter uint64

func CreateStat() *Stat {
	s := &Stat{
		Start:          time.Now(),
		RequestsTotal:  0,
		RequestsServed: make(map[int]int),
		LengthServed:   0,
		SitesStat:      make(map[string]*SiteStat),
	}
	for _, host := range config.Config.Hosts {
		s.SitesStat[host.Name] = &SiteStat{
			RequestsServed: make(map[int]int),
		}
	}

	// launch cleaning thread, while the xamboo go system works
	go s.Clean()

	return s
}

func Start() {
	SystemStat = CreateStat()
}

func (s *Stat) Clean() {
	// 1. clean Requests from stat
	slogger := logger.GetCoreLogger("sys")
	slogger.Println("Stats cleaner launched. Clean every minute.")
	for {
		n := time.Now()
		// we keep 2 minutes
		delta := time.Minute * 2
		last := 0

		// if it's alive: no delete
		s.mutex.Lock()
		for i, r := range s.Requests {
			if r.Time.Add(delta).Before(n) {
				last = i
			} else {
				break
			}
		}
		s.Requests = s.Requests[last:]
		s.mutex.Unlock()
		// we clean every 60 seconds
		time.Sleep(time.Minute)
	}
}

func CreateRequestStat(request string, method string, protocol string, code int, length int, duration time.Duration, remoteaddr string) *RequestStat {

	ip, port, _ := net.SplitHostPort(remoteaddr)

	r := &RequestStat{
		Id:        RequestCounter,
		StartTime: time.Now(),
		Time:      time.Now(),
		Request:   request,
		Method:    method,
		Protocol:  protocol,
		Code:      code,
		Length:    length,
		Duration:  duration,
		IP:        ip,
		Port:      port,
		Alive:     true,
	}
	RequestCounter++

	SystemStat.LengthServed += length

	SystemStat.mutex.Lock()
	SystemStat.RequestsTotal++
	SystemStat.Requests = append(SystemStat.Requests, r)
	SystemStat.mutex.Unlock()

	// Adding stat to the site:
	return r
}

func (r *RequestStat) UpdateStat(code int, length int) {
	r.Time = time.Now()
	if code != 0 {
		r.Code = code
	}
	r.Length += length
	SystemStat.LengthServed += length
	r.Duration = r.Time.Sub(r.StartTime)

	// Put the stat at the end of the pile.. it has been modified!
	SystemStat.mutex.Lock()
	// find the request. It is highly possible it's at the end of Pile
	for i := len(SystemStat.Requests) - 1; i >= 0; i-- {
		if SystemStat.Requests[i] == r {
			if i == len(SystemStat.Requests)-1 {
				SystemStat.Requests = SystemStat.Requests[:i]
			} else {
				SystemStat.Requests = append(SystemStat.Requests[:i], SystemStat.Requests[i+1:]...)
			}
			break
		}
	}
	SystemStat.Requests = append(SystemStat.Requests, r)
	SystemStat.mutex.Unlock()
}

func (r *RequestStat) UpdateProtocol(protocol string) {
	r.Protocol = protocol
}

func (r *RequestStat) End() {

	// Call stats ? (code entry)
	// log the stat in pages and stat loggers
	if r.Hostname == "" {
		xlogger := logger.GetCoreLogger("errors")
		xlogger.Println("Stat without hostname:", r.IP, r.Method, r.Protocol, r.Code, r.Request, r.Length, r.Duration)
	} else {
		hlogger := logger.GetHostLogger(r.Hostname, "pages")
		slogger := logger.GetHostHook(r.Hostname, "stats")
		if hlogger != nil {
			hlogger.Println(r.IP, r.Method, r.Protocol, r.Code, r.Request, r.Length, r.Duration)
		}
		if slogger != nil && r.Context != nil {
			slogger(r.Context)
		}
	}

	// closed case
	r.Alive = false
}
