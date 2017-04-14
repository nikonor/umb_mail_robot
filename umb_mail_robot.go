package main

import (
	"encoding/json"
	"fmt"
	uDB "github.com/nikonor/umb_db"
	u "github.com/nikonor/umb_lib"
	// "strings"
	"errors"
	"math"
	// "io/ioutil"
	"log"
	"os"
	"time"
	// "reflect"
	// T "github.com/jmoiron/sqlx/types"
	// "strconv"
)

const (
	WORKERS int = 3
)

var (
	err error
	// tx *sql.Tx
	Conf   map[string]string
	mdb    uDB.MyDB
	log_fh *os.File
)

type Mail4Send struct {
	Id                   int64
	From, To, Subj, Body string
	AttachFiles          []u.AttachFile
	AttachDocs           []u.AttachDoc
}

func (m Mail4Send) String() string {
	ret := fmt.Sprintf("ID=%d\nFrom: %s\nTo: %s\nSubj: %s\nBody: %s...\n", m.Id, m.From, m.To, m.Subj, m.Body[0:32])
	if len(m.AttachDocs) > 0 {
		ret = ret + "AttachDoc\n"
		for _, f := range m.AttachDocs {
			ret = ret + fmt.Sprintf("\tID=%d,Type=%d\n", f.Doc_id, f.Type)
		}
	}
	if len(m.AttachFiles) > 0 {
		ret = ret + "AttachFiles\n"
		for _, f := range m.AttachFiles {
			exist := "No"
			if f.Body == nil {
				exist = "Yes"
			}
			ret = ret + fmt.Sprintf("\tName=%s,Exist=%s\n", f.Name, exist)
		}
	}
	return ret + "\n"
}

func init() {
	// создает каталог для конфигов
	// os.Mkdir(fmt.Sprintf("%s/umb_mail_robot",os.TempDir()),0777)
	// TK
	Conf = u.ReadConf("")

	// проверяем наличие pid-файла и если он есть, то выходим
	if u.CheckPidFile("umb_mail_robot") {
		// fmt.Printf("PidFile exist => exit\n")
		os.Exit(1)
	}

}

func Work(Conf map[string]string, mdb uDB.MyDB, jobs <-chan Mail4Send, results chan<- error) {
	// var C map[string]string

	for m := range jobs {
		log.Println("Start for ID=", m.Id, ", Subj=", m.Subj, ", To=", m.To, ", From=", m.From)
		C, _ := u.GetEMailConf(Conf, m.From)
		if C["login"] == "" {
			log.Println("Unknown sender: ", m.From)
			results <- errors.New(fmt.Sprintf("Unknown sender: %s\n", m.From))
		} else {
			tmpdir := fmt.Sprintf("%s/mail%d", os.TempDir(), m.Id)
			os.Mkdir(tmpdir, 0777)
			// не анализируем ошибку, чтобы в следующий раз м.б. не вываливаться, если что-то пошло не так.
			// if err = ; err != nil {
			// 	log.Println("Error:",err)
			// 	results <-  err
			// }

			if err := u.SendEMail(m.Id, C, m.From, m.To, m.Subj, m.Body, true, m.AttachFiles, m.AttachDocs); err != nil {
				log.Println("Error:", err)
				results <- err
			} else {
				// TK
				res, err := mdb.DB.Exec("update mail set sent='t' where id=$1", m.Id)
				ra, err2 := res.RowsAffected()
				if err != nil || err2 != nil || ra != 1 {
					log.Println("Error:", err)
					results <- errors.New(fmt.Sprintf("Ошибка отметки письма, как отправленного. Id=%d\n", m.Id))
				}
			}
			// заменили на RemoveAll, чтобы удалтья rm -rf
			if err = os.RemoveAll(tmpdir); err != nil {
				log.Println("Error:", err)
				results <- (err)
			}
			log.Println("Ok for ID=", m.Id)
			results <- nil
		}
	}
}

func main() {
	// файл для логов
	log_fh, err := os.OpenFile(fmt.Sprintf("/tmp/%s_gomail.log", time.Now().Format("2006-01-02")), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("error opening file: %v", err)
	}
	defer log_fh.Close()
	log.SetOutput(log_fh)

	// связываемся с базой
	if mdb, err = uDB.BeginDB(Conf); err != nil {
		log.Printf("umb_mail_robot::BeginDB::%s", err)
	}
	defer mdb.CloseDB()

	// получаем неотправленные письма
	mails := getUnsentEMails(mdb)
	fmt.Printf("%v\n", mails)

	// создаем каналы
	jobs := make(chan Mail4Send, len(mails))
	results := make(chan error, len(mails))

	// запускаем воркеров
	for w := 1; w <= WORKERS; w++ {
		go Work(Conf, mdb, jobs, results)
	}

	// отправляем письма в очередь на отправку
	for _, m := range mails {
		jobs <- m
	}
	close(jobs)

	// смотрим, что получилось
	for i := 0; i < len(mails); i++ {
		err := <-results
		if err != nil {
			fmt.Println("Error:", err)
		}
	}
	os.Remove(fmt.Sprintf("/tmp/%s", "umb_mail_robot.pid"))
}

//  Получаем письма, которые надо отправить
func getUnsentEMails(db uDB.MyDB) []Mail4Send {
	mails := []Mail4Send{}
	//                0    1          2          3        4                      5
	sel := `SELECT m.id,m.sender,m.reciever,m.subject,m.email_text,
					array_to_json(array(select row_to_json(mail_att) from mail_att where mail_id=m.id)) 
			from mail m 
			where m.sent='f'
			order by m.id
	`
	if mm, err := mdb.Rows(sel, []interface{}{}); err != nil {
		log.Printf("Error on Rows(%s)::%s", sel, err)
	} else {
		for _, m := range mm {
			mail, err := parseMailFromDB(m, db)
			if err == nil {
				mails = append(mails, mail)
			} else {
				fmt.Println(err)
			}
		}
	}

	return mails
}

// Разбираем то, что получили из базы и кладем по местам
func parseMailFromDB(row []interface{}, db uDB.MyDB) (Mail4Send, error) {
	var r5 []map[string]interface{}
	mail := Mail4Send{}

	mail.Id = row[0].(int64)
	mail.From = row[1].(string)
	mail.To = row[2].(string)
	mail.Subj = row[3].(string)
	mail.Body = row[4].(string)

	if mail.Id == 0 || math.Abs(float64(mail.Id*(-1))) != float64(mail.Id) || len(mail.From) == 0 || len(mail.To) == 0 {
		return mail, errors.New("Не удалось получить все необходимые поля")
	}

	mail.AttachDocs = []u.AttachDoc{}
	mail.AttachFiles = []u.AttachFile{}

	err := json.Unmarshal([]byte(string(row[5].(string))), &r5)
	if err != nil {
		return mail, err
	}

	for _, a := range r5 {
		if a["doc_id"] != nil {
			new_attach_doc := u.AttachDoc{(int64(a["doc_id"].(float64))), "", (int64(a["doc_type"].(float64)))}
			new_attach_doc.Name = fmt.Sprintf("%s_%d.pdf", "document", int64(a["doc_id"].(float64)))
			mail.AttachDocs = append(mail.AttachDocs, new_attach_doc)
		}
		if a["attachment"] != nil {
			filename := fmt.Sprintf("%s/mail%d/%s", os.TempDir(), mail.Id, a["name"])
			if filebody, err := db.Row0("select encode(attachment,'escape') from mail_att where id=$1", []interface{}{int64(a["id"].(float64))}); err != nil {
				log.Printf("Error on Row0(%s)::%s", "select encode(attachment,'escape') from mail_att where id=", err)
			} else {
				mail.AttachFiles = append(mail.AttachFiles, u.AttachFile{filename, []byte(filebody.(string))})
			}
		}
		if a["attachment"] == nil && a["doc_id"] == nil && a["name"] != nil {
			mail.AttachFiles = append(mail.AttachFiles, u.AttachFile{a["name"].(string), nil})
		}
	}

	return mail, nil
}
