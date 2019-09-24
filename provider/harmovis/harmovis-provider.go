package main

import (
	"context"
	"flag"
	"fmt"
	"time"
	"io/ioutil"
	"github.com/mtfelian/golang-socketio"
	"github.com/mtfelian/golang-socketio/transport"
	"github.com/synerex/synerex_alpha/api"
	"github.com/synerex/synerex_alpha/sxutil"
	"google.golang.org/grpc"
	"log"
	"net/http"
	"os"
	"encoding/json"
	"path/filepath"
	"runtime"
	"sync"
)

// Harmoware Vis-Synerex provider provides map information to Web Service through socket.io.

var (
	serverAddr = flag.String("server_addr", "127.0.0.1:10000", "The server address in the format of host:port")
	nodesrv    = flag.String("nodesrv", "127.0.0.1:9990", "Node ID Server")
	port       = flag.Int("port", 10080, "HarmoVis Provider Listening Port")
	mu         sync.Mutex
	version    = "0.01"
	assetsDir  http.FileSystem
	ioserv     *gosocketio.Server
)


func toJSON(m map[string]interface{}, utime int64) string {
	s := fmt.Sprintf("{\"mtype\":%d,\"id\":%d,\"time\":%d,\"lat\":%f,\"lon\":%f,\"angle\":%f,\"speed\":%d}",
		0, int(m["vehicle_id"].(float64)),utime, m["coord"].([]interface{})[0].(float64), m["coord"].([]interface{})[1].(float64), m["angle"].(float64), int(m["speed"].(float64)))
	return s
}

func handleFleetMessage(sv *gosocketio.Server, param interface{}){
	var  bmap map[string]interface{}
	utime := time.Now().Unix()
	bmap = param.(map[string]interface{})
	for _, v := range bmap["vehicles"].([]interface{}){
		m, _ := v.(map[string]interface{})
		s := toJSON(m, utime)
		sv.BroadcastToAll("event", s)
	}
}


func getFleetInfo(serv string, sv *gosocketio.Server, ch chan error){
	fmt.Printf("Dial to [%s]\n", serv)
	sioClient, err := gosocketio.Dial(serv + "socket.io/?EIO=3&transport=websocket", transport.DefaultWebsocketTransport())
	if err != nil{
		log.Printf("SocketIO Dial error: %s",err)
		return
	}

	sioClient.On(gosocketio.OnConnection, func(c *gosocketio.Channel, param interface{}){
		fmt.Println("Fleet-Provider socket.io connected ", c)
	})
		
	sioClient.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel, param interface{}){
		fmt.Println("Fleet-Provider socket.io disconnected ", c)
		ch <- fmt.Errorf("Disconnected!\n")
	})

	sioClient.On("vehicle_status",  func(c *gosocketio.Channel, param interface{}){
		handleFleetMessage(sv, param)
	})
	
}


func runFleetInfo(serv string, sv *gosocketio.Server){
	ch := make(chan error)
	for {
		time.Sleep(3 * time.Second)
		getFleetInfo(serv, sv, ch)
		res := <-ch
		if res == nil {
			break
		}
	}
}

// assetsFileHandler for static Data
func assetsFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return
	}

	file := r.URL.Path
	//	log.Printf("Open File '%s'",file)
	if file == "/" {
		file = "/index.html"
	}
	f, err := assetsDir.Open(file)
	if err != nil {
		log.Printf("can't open file %s: %v\n", file, err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Printf("can't open file %s: %v\n", file, err)
		return
	}
	http.ServeContent(w, r, file, fi.ModTime(), f)
}

func runServer() *gosocketio.Server {

	currentRoot, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	d := filepath.Join(currentRoot, "mclient", "build")

	assetsDir = http.Dir(d)
	log.Println("AssetDir:", assetsDir)

	assetsDir = http.Dir(d)
	server := gosocketio.NewServer()

	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel) {
		log.Printf("Connected from %s as %s", c.IP(), c.Id())
		// do something.
	})

	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		log.Printf("Disconnected from %s as %s", c.IP(), c.Id())
	})

	return server
}

// MapMarker struct
type MapMarker struct {
	mtype int32   `json:"mtype"`
	id    int32   `json:"id"`
	lat   float32 `json:"lat"`
	lon   float32 `json:"lon"`
	angle float32 `json:"angle"`
	speed int32   `json:"speed"`
}

// GetJSON get json from MapMarker
func (m *MapMarker) GetJSON() string {
	// s := fmt.Sprintf("{\"mtype\":%d,\"id\":%d,\"lat\":%f,\"lon\":%f,\"angle\":%f,\"speed\":%d}",
	//	m.mtype, m.id, m.lat, m.lon, m.angle, m.speed)
	byteJSON, err := json.Marshal(m)
	if (err != nil) {
		log.Printf("fail to parse MapMarker %s", err.Error())
	}
	return string(byteJSON)
}

func supplyRideCallback(clt *sxutil.SMServiceClient, sp *api.Supply) {
	flt := sp.GetArg_Fleet()
	if flt != nil { // get Fleet supplu
		mm := &MapMarker{
			mtype: int32(0),
			id:    flt.VehicleId,
			lat:   flt.Coord.Lat,
			lon:   flt.Coord.Lon,
			angle: flt.Angle,
			speed: flt.Speed,
		}
		//jsondata, err := json.Marshal(*mm)
		fmt.Println("rcb",mm.GetJSON())
		mu.Lock()
		ioserv.BroadcastToAll("event", mm.GetJSON())
		mu.Unlock()
	}
}

func subscribeRideSupply(client *sxutil.SMServiceClient) {
	ctx := context.Background() //
	err := client.SubscribeSupply(ctx, supplyRideCallback)
	log.Printf("Error:Supply %s\n",err.Error())
}

func supplyPTCallback(clt *sxutil.SMServiceClient, sp *api.Supply) {
	pt := sp.GetArg_PTService()
	if pt != nil { // get Fleet supplu
		mm := &MapMarker{
			mtype: pt.VehicleType, // depends on type of GTFS: 1 for Subway, 2, for Rail, 3 for bus
			id:    pt.VehicleId,
			lat:   float32(pt.CurrentLocation.GetPoint().Latitude),
			lon:   float32(pt.CurrentLocation.GetPoint().Longitude),
			angle: pt.Angle,
			speed: pt.Speed,
		}
		mu.Lock()
		ioserv.BroadcastToAll("event", mm.GetJSON())
		mu.Unlock()
	}
}

func subscribePTSupply(client *sxutil.SMServiceClient) {
	ctx := context.Background() //
	err := client.SubscribeSupply(ctx, supplyPTCallback)
	log.Printf("Error:Supply %s\n",err.Error())
}

func monitorStatus(){
	for{
		sxutil.SetNodeStatus(int32(runtime.NumGoroutine()),"MapGoroute")
		time.Sleep(time.Second * 3)
	}
}

type RainfallData struct {
	Elevation float64 `json:"elevation"`
	Position []float64 `json:"position"`
	Color []int `json:"color"`
}

func scanRainfallDataFrom(dir string) []string {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
			panic(err)
	}
	var paths []string
	for _, file := range files {
			paths = append(paths, filepath.Join(dir, file.Name()))
	}
	return paths
}

func startProvidingRainFallData (ctx context.Context) {
	fmt.Println("start providing rain fall data")
	ticker := time.NewTicker(time.Second)
	files := scanRainfallDataFrom("./rainfall-data")
	maxFileIndex := len(files) - 1
	fileIndex := 0
	for {
		select {
		case <-ticker.C:
			fileIndex++
			if (fileIndex > maxFileIndex) {
				fileIndex = 0
			}
			file := files[fileIndex]
			log.Printf("load rainfall data from %s\n", file)
			rainfallJSON, err := ioutil.ReadFile(file)
			if err != nil {
					panic(err)
			}
			provideRainData(string(rainfallJSON))
		case <-ctx.Done():
			return
		}
	}
}

func provideRainData(rainfallJSON string) {
	ioserv.BroadcastToAll("notify_new_rainfall_data", rainfallJSON)
}

func main() {
	flag.Parse()
	sxutil.RegisterNodeName(*nodesrv, "HarmoProvider", false)

	go sxutil.HandleSigInt()
	sxutil.RegisterDeferFunction(sxutil.UnRegisterNode)

	var opts []grpc.DialOption
	wg := sync.WaitGroup{} // for syncing other goroutines

	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("Fail to Connect Synerex Server: %v", err)
	}
	ioserv = runServer()
	fmt.Printf("Running HarmoVis Server..\n")
	if ioserv == nil {
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go startProvidingRainFallData(ctx)
	defer cancel()

	client := api.NewSynerexClient(conn)
	argJSON := fmt.Sprintf("{Client:Map:RIDE}")
	rideClinet := sxutil.NewSMServiceClient(client, api.ChannelType_RIDE_SHARE, argJSON)

	argJSON2 := fmt.Sprintf("{Client:Map:PT}")
	ptClient := sxutil.NewSMServiceClient(client, api.ChannelType_PT_SERVICE, argJSON2)

	wg.Add(1)
	go subscribeRideSupply(rideClinet)
	wg.Add(1)
	go subscribePTSupply(ptClient)

	go monitorStatus() // keep status

	serveMux := http.NewServeMux()

	serveMux.Handle("/socket.io/", ioserv)
	serveMux.HandleFunc("/", assetsFileHandler)

	log.Printf("Starting Harmoware VIS  Provider %s  on port %d", version, *port)
	err = http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *port), serveMux)
	if err != nil {
		log.Fatal(err)
	}

	wg.Wait()

}
