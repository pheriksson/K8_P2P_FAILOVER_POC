package main

import (
	"flag"
	"fmt"
	"io/ioutil"
//	"log"
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
	FailedRequestsMut sync.Mutex
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
		}else{
		        reqStat.ElapsedTime = reqStat.RespTime.Sub(reqStat.SentTime) 
		}
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
			res+= fmt.Sprintf("[%d] starttime: [%s], recievedresp: TRUE, resptime: [%s], elapsedtime: [%d(ms)]\n", req.RequestId, req.SentTime, req.RespTime, req.ElapsedTime.Milliseconds())
		}else{
			res+= fmt.Sprintf("[%d] starttime: [%s], recievedresp: FALSE, resptime: [NA], elapsedtime: [NA]\n", req.RequestId, req.SentTime)
		}
	}
	return res
}

func (td *TestData) getAllRequestsDataSlim() (string){
	res := ""
	for _, req := range td.Requests{
		if req.Response != ""{
			res+= fmt.Sprintf("[%d] recievedresp: TRUE, elapsedtime: [%d(ms)]\n", req.RequestId, req.ElapsedTime.Milliseconds())
		}else{
			res+= fmt.Sprintf("[%d] recievedresp: FALSE, elapsedtime: [NA]\n", req.RequestId)
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
					//log.Println("FAILED RESPONSE, err:", err.Error())
					waitGroup.Done()
					return
				}
				reqData.RespTime = time.Now()
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				bodyString := string(body)
				reqData.Response = bodyString
				//fmt.Println("RECIEVED:", bodyString)
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



func main(){
	var ip, port string
	var hz, cycles int
	flag.StringVar(&ip, "ip", "", "ip address for target")
	flag.StringVar(&port,"port", "80", "port for target - no port will default to 80")
	flag.IntVar(&hz, "hz", 20, "number of calls per sec (Hz)")
	flag.IntVar(&cycles, "cycles", 4, "number iterations of num freq")
	flag.Parse()

	data := startNSTest(ip, port, cycles, hz)
	_, summary := data.getSummary()
	fmt.Println("TEST RESULTS:\n", summary)
	//allQueries := data.getAllRequestsData()
	allQueries := data.getAllRequestsDataSlim()
	fmt.Println("ALL DATA:\n", allQueries)
}
