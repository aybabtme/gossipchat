package main

import (
	"encoding/json"
	"github.com/pborman/uuid"
	"log"
	"sort"
	"sync"
	"time"
)

type message struct {
	Time   time.Time `json:"-"`
	UUID   string    `json:"uuid"`
	Author string    `json:"author"`
	Msg    string    `json:"msg"`
}

type byTime []message

func (b byTime) Len() int           { return len(b) }
func (b byTime) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byTime) Less(i, j int) bool { return b[i].Time.Before(b[j].Time) }

func newMessage(author, msg string) message {
	return message{
		Time:   time.Now(),
		UUID:   uuid.New(),
		Author: author,
		Msg:    msg,
	}
}

func (msg message) Encode() []byte {
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}

func (msg *message) Decode(data []byte) error {
	return msg.UnmarshalJSON(data)
}

func (msg *message) UnmarshalJSON(data []byte) error {
	v := struct {
		UUID   string
		Author string
		Msg    string
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	msg.Time = time.Now()
	msg.UUID = v.UUID
	msg.Author = v.Author
	msg.Msg = v.Msg
	return nil
}

type messageDB struct {
	mu         sync.Mutex
	maxHistory int
	uuidToMsg  map[string]message

	sortedCache []message
}

func newMessageDB(maxHistory int) *messageDB {
	if maxHistory < 1 {
		panic("can't have no message history")
	}
	return &messageDB{
		maxHistory: maxHistory,
		uuidToMsg:  make(map[string]message),
	}
}

func encodeMessageDB(db *messageDB) []byte {
	var export = struct {
		Messages []message
	}{db.Messages()}
	data, err := json.Marshal(export)
	if err != nil {
		panic("can't encode struct messageDB: " + err.Error())
	}
	return data
}

func (db *messageDB) mergeRemoteDB(data []byte) error {
	var export = struct {
		Messages []message
	}{}
	err := json.Unmarshal(data, &export)
	if err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, msg := range export.Messages {
		db.put(msg)
	}
	return nil
}

func (db *messageDB) Put(msg message) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.put(msg)
	log.Printf("got %d messages", len(db.sortedCache))
}

func (db *messageDB) Messages() []message {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.sortedCache
}

func (db *messageDB) put(msg message) {
	if _, ok := db.uuidToMsg[msg.UUID]; ok {
		// first write wins
		return
	}
	log.Printf("dont know about %q", msg.UUID)

	if len(db.uuidToMsg) >= db.maxHistory &&
		msg.Time.Before(db.sortedCache[0].Time) {
		// too old to care
		return
	}

	db.uuidToMsg[msg.UUID] = msg
	db.sortedCache = append(db.sortedCache, msg)
	sort.Sort(byTime(db.sortedCache))

	if len(db.uuidToMsg) <= db.maxHistory {
		return
	}

	extra := len(db.sortedCache) - db.maxHistory
	toDelete := db.sortedCache[:extra]
	db.sortedCache = db.sortedCache[extra:]
	for _, msg := range toDelete {
		delete(db.uuidToMsg, msg.UUID)
	}
}
