package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"net/http"
	"sort"
	"sync"
	"time"
)

type RequestStats struct{
	RequestId int
	SentTime time.Time
	RespTime time.Time
	ElapsedTime time.Duration
	Response string
	Cluster string
	Hostname string
}

func InitStatsContainer(id int) *RequestStats{
	r := &RequestStats{
		RequestId: id,
		SentTime: time.Now(),
	}
	return  r
}

type TestData struct{
	StartTime time.Time
	StopTime time.Time
	Duration time.Duration
	Requests []*RequestStats
	TotalRequests int
	FailedRequests int
	Cycles int
	Hz int
}

func InitTestData(cycles int, hz int) *TestData{
	return &TestData{
		TotalRequests: cycles*hz,
		Cycles: cycles,
		Hz: hz,
		FailedRequests: 0,
	}
}

func (td *TestData) startTestData(){
	td.StartTime = time.Now()
}

func (td *TestData) stopTestData(){
	td.StopTime = time.Now()
	td.Duration = td.StopTime.Sub(td.StartTime)
	for _, reqStat := range td.Requests {
		if reqStat.Response == ""{
			td.FailedRequests+=1
			continue
		}else{
			values := parseServerMsgValues(reqStat.Response)
			reqStat.Cluster = values[0]
			reqStat.Hostname = values[1]
		}
		reqStat.ElapsedTime = reqStat.RespTime.Sub(reqStat.SentTime) 
	}


	td.sortDataSet()
}

func (td *TestData) sortDataSet(){
	sort.Slice(td.Requests, func (i, j int) bool {
		return td.Requests[i].SentTime.Before(td.Requests[j].SentTime)
	})
}


func (td *TestData) getSummary() (error, string){
	if td.StopTime.IsZero() {
		return fmt.Errorf("TEST NOT COMLETED SUCCESFULLY"), ""
	}
	elapsedTime := fmt.Sprintf("elapsed_time: [%s]", td.StopTime.Sub(td.StartTime).String())
	numberOfRequests := fmt.Sprintf("num_requests_sent: [%d]", td.TotalRequests) 
	numberOfRequestsFailed := fmt.Sprintf("num_requests_failed: [%d]", td.FailedRequests)

	return nil, elapsedTime+"\n"+numberOfRequests+"\n"+numberOfRequestsFailed
}

func (td *TestData) getAllRequestsData() (string){
	res := ""
	for _, req := range td.Requests{
		if req.Response != ""{
			res+= fmt.Sprintf("(%d), cluster: [%s], starttime: [%s], recievedresp: TRUE, resptime: [%s], elapsedtime: [%d(ms)]\n", req.RequestId, req.Cluster, req.SentTime, req.RespTime, req.ElapsedTime.Milliseconds())
		}else{
			res+= fmt.Sprintf("(%d), cluster: [%s], starttime: [%s], recievedresp: FALSE, resptime: [NA], elapsedtime: [NA]\n", req.RequestId, req.Cluster, req.SentTime)
		}
	}
	return res
}

func (td *TestData) getAllRequestsDataSlim() (string){
	res := ""
	for _, req := range td.Requests{
		if req.Response != ""{
			res+= fmt.Sprintf("(%d) cluster: [%s], recievedresp: TRUE, elapsedtime: [%d(ms)]\n", req.RequestId, req.Cluster, req.ElapsedTime.Milliseconds())
		}else{
			res+= fmt.Sprintf("(%d) cluster: [%s], recievedresp: FALSE, elapsedtime: [NA]\n", req.RequestId, req.Cluster)
		}
	}
	return res
}

func (td *TestData) getAllRequestsCompact() (string){
	res := ""
	for _, req := range td.Requests{
		if req.Response != ""{
			res+= fmt.Sprintf("[%d],[%s],[%d]\n", req.RequestId, req.Cluster, req.ElapsedTime.Milliseconds())
		}else{
			res+= fmt.Sprintf("[%d],[%s],[NA]\n", req.RequestId, req.Cluster)
		}
	}
	return res
}



func startNSTest(ip string, port string, cycles int, hz int) *TestData{
	addr := "http://"+ip+":"+port
	fmt.Println(fmt.Sprintf("STARTING TEST FOR [%s] WITH NUM CYCLES: [%d] AND HZ [%d]", addr, cycles, hz))

	stats := InitTestData(cycles, hz) 
	stats.startTestData()
	for n := 0; n < cycles; n++{
		wg := &sync.WaitGroup{}
		wg.Add(hz)
		for call := 0; call < hz; call++{
			requestContainer := InitStatsContainer(n*hz+call)
			go func(waitGroup *sync.WaitGroup, reqData *RequestStats){
				resp, err := http.Get(addr)
				if err != nil {
					waitGroup.Done()
					return
				}
				reqData.RespTime = time.Now()
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				bodyString := string(body)
				reqData.Response = bodyString
				waitGroup.Done()
			}(wg, requestContainer)
			stats.Requests = append(stats.Requests, requestContainer)
		}
		wg.Wait()
		fmt.Println(fmt.Sprintf("Finished cycle [%d/%d]", n+1, cycles))
	}
	stats.stopTestData()
	return stats
}

// Currently parsing 'CLUSTER_ID: %s & HOSTNAME: %s ' -> {%s,%s}
func parseServerMsgValues(msg string) []string{
	tuples := strings.Split(msg, "&")
	res := []string{}

	for _, tuple := range tuples{
		keyValue := strings.Split(tuple, ":")
		if len(keyValue) !=  2 {
			log.Panic("CANNOT PARSE SERVER RESPONSE")
		}
		keyValue[0] = strings.TrimSpace(keyValue[0])
		keyValue[1] = strings.TrimSpace(keyValue[1])
		res = append(res, keyValue[1])
	} 
	return res 
}

func main(){
	var ip, port string
	var hz, cycles int
	flag.StringVar(&ip, "ip", "", "ip address for target")
	flag.StringVar(&port,"port", "80", "port for target")
	flag.IntVar(&hz, "hz", 20, "number of calls per sec (Hz)")
	flag.IntVar(&cycles, "cycles", 1, "number iterations of num freq")
	flag.Parse()

	data := startNSTest(ip, port, cycles, hz)
	//allQueries := data.getAllRequestsData()
	//allQueries := data.getAllRequestsDataSlim()
	allQueries := data.getAllRequestsCompact()
	_, summary := data.getSummary()
	fmt.Println("TEST RESULTS:\n", summary)
	fmt.Println("ALL DATA:\n", allQueries)
}
