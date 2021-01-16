package ctrl

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"gopkg.in/fatih/set.v0"
)

const (
	CMD_SINGLE_MSG = 10
	CMD_ROOM_MSG   = 11
	CMD_HEART      = 0
)

type Message struct {
	Id      int64  `json:"id,omitempty" form:"id"`
	Userid  int64  `json:"userid,omitempty" form:"userid"`
	Cmd     int    `json:"cmd,omitempty" form:"cmd"`
	Dstid   int64  `json:"dstid,omitempty" form:"dstid"`
	Media   int    `json:"media,omitempty" form:"media"`
	Content string `json:"content,omitempty" form:"content"`
	Pic     string `json:"pic,omitempty" form:"pic"`
	Url     string `json:"url,omitempty" form:"url"`
	Memo    string `json:"memo,omitempty" form:"memo"`
	Amount  int    `json:"amount,omitempty" form:"amount"`
}

type Node struct {
	Conn      *websocket.Conn
	DataQueue chan []byte
	GroupSets set.Interface
}

var clientMap map[int64]*Node = make(map[int64]*Node, 0)

var rwlocker sync.RWMutex

func Chat(writer http.ResponseWriter,
	request *http.Request) {

	query := request.URL.Query()
	id := query.Get("id")
	token := query.Get("token")
	userId, _ := strconv.ParseInt(id, 10, 64)
	isvalida := checkToken(userId, token)

	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return isvalida
		},
	}).Upgrade(writer, request, nil)
	if err != nil {
		log.Println(err.Error())
		return
	}

	node := &Node{
		Conn:      conn,
		DataQueue: make(chan []byte, 50),
		GroupSets: set.New(set.ThreadSafe),
	}

	comIds := contactService.SearchComunityIds(userId)
	for _, v := range comIds {
		node.GroupSets.Add(v)
	}

	rwlocker.Lock()
	clientMap[userId] = node
	rwlocker.Unlock()

	go sendproc(node)

	go recvproc(node)
	log.Printf("<-%d\n", userId)
	sendMsg(userId, []byte("hello,world!"))
}

func AddGroupId(userId, gid int64) {

	rwlocker.Lock()
	node, ok := clientMap[userId]
	if ok {
		node.GroupSets.Add(gid)
	}

	rwlocker.Unlock()

}

func sendproc(node *Node) {
	for {
		select {
		case data := <-node.DataQueue:
			err := node.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Println(err.Error())
				return
			}
		}
	}
}

func recvproc(node *Node) {
	for {
		_, data, err := node.Conn.ReadMessage()
		if err != nil {
			log.Println(err.Error())
			return
		}

		broadMsg(data)
		log.Printf("[ws]<=%s\n", data)
	}
}

func init() {
	go udpsendproc()
	go udprecvproc()
}

var udpsendchan chan []byte = make(chan []byte, 1024)

func broadMsg(data []byte) {
	udpsendchan <- data
}

func udpsendproc() {
	log.Println("start udpsendproc")

	con, err := net.DialUDP("udp", nil,
		&net.UDPAddr{
			IP:   net.IPv4(192, 168, 0, 255),
			Port: 3000,
		})
	defer con.Close()
	if err != nil {
		log.Println(err.Error())
		return
	}
	for {
		select {
		case data := <-udpsendchan:
			_, err = con.Write(data)
			if err != nil {
				log.Println(err.Error())
				return
			}
		}
	}
}

func udprecvproc() {
	log.Println("start udprecvproc")

	con, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 3000,
	})
	defer con.Close()
	if err != nil {
		log.Println(err.Error())
	}

	for {
		var buf [512]byte
		n, err := con.Read(buf[0:])
		if err != nil {
			log.Println(err.Error())
			return
		}

		dispatch(buf[0:n])
	}
	log.Println("stop updrecvproc")
}

func dispatch(data []byte) {

	msg := Message{}
	err := json.Unmarshal(data, &msg)
	if err != nil {
		log.Println(err.Error())
		return
	}

	switch msg.Cmd {
	case CMD_SINGLE_MSG:
		sendMsg(msg.Dstid, data)
	case CMD_ROOM_MSG:

		for _, v := range clientMap {
			if v.GroupSets.Has(msg.Dstid) {
				v.DataQueue <- data
			}
		}
	case CMD_HEART:

	}
}

func sendMsg(userId int64, msg []byte) {
	rwlocker.RLock()
	node, ok := clientMap[userId]
	rwlocker.RUnlock()
	if ok {
		node.DataQueue <- msg
	}
}

func checkToken(userId int64, token string) bool {

	user := userService.Find(userId)
	return user.Token == token
}
