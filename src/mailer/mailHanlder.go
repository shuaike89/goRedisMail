package mailer

import (
	"github.com/go-gomail/gomail"
	"sync"
	"time"
)

type TMailMessageItem struct {
	To []string
	Subject string
	Content string
	Time time.Time
	FailNum int
}

type TMailHandler struct {
	sync.Mutex
	stopChannel chan int
	mailMessageItemChannel chan *TMailMessageItem
	mailSender gomail.SendCloser
	mailMessage *gomail.Message
}

func (obj *TMailHandler) Init() {
	obj.stopChannel = make(chan int, 0)
	obj.mailMessageItemChannel = make(chan *TMailMessageItem, 1000)
	obj.mailSender = nil
	obj.mailMessage = nil

	go func() {
		checkInterval := time.NewTicker(time.Second * CHECK_MAIL_CONNECTION_STATE_INTERVAL_SECONDS)
		defer func() {
			Logger.Print("mailHandler will stop")
			checkInterval.Stop()
			close(obj.mailMessageItemChannel)
			obj.stopChannel <- 1
		}()

	E:
		for {
			select {
			case <-checkInterval.C:
				obj.tryCloseConnectedMailServer()
			case mailMessageItem := <-obj.mailMessageItemChannel:
				Logger.Printf("got mail message %v", mailMessageItem)
				obj.SenderMail(mailMessageItem)
			case <-obj.stopChannel:
				Logger.Print("mailHandler catch stop signal")
				break E
			}
		}

	F:
		for {
			select {
			case mailMessageItem := <-obj.mailMessageItemChannel:
				Logger.Printf("got mail message %v", mailMessageItem)
				obj.SenderMail(mailMessageItem)
			default:
				break F
			}
		}
	}()
}

func (obj *TMailHandler) tryCloseConnectedMailServer() {
	obj.Lock()
	defer obj.Unlock()

	if obj.mailSender != nil {
		err := obj.mailSender.Close()
		if err != nil {
			Logger.Print(err)
		}
	}

	obj.mailSender = nil
}

func (obj *TMailHandler) SenderMail(mailMessageItem *TMailMessageItem) {
	obj.Lock()
	defer obj.Unlock()

	senderMailOK := false

	defer func() {
		if senderMailOK {
			return
		}

		if mailMessageItem.FailNum < 3 {
			mailMessageItem.FailNum++
			time.Sleep(time.Second * 3)
			obj.mailMessageItemChannel <- mailMessageItem
		} else {
			Logger.Printf("send mail %v failed overflow max num", mailMessageItem)
		}
	}()

	if obj.mailSender == nil {
		mailDialer := gomail.NewDialer(GConfig.MailHost, GConfig.MailPort, GConfig.MailUser, GConfig.MailPassword)
		mailSender, err := mailDialer.Dial()
		if err != nil {
			Logger.Print(err)
			return
		}
		obj.mailSender = mailSender
	}

	if obj.mailMessage == nil {
		obj.mailMessage = gomail.NewMessage()
	}

	obj.mailMessage.SetHeader("From", GConfig.MailUser)
	obj.mailMessage.SetHeader("To", mailMessageItem.To...)
	obj.mailMessage.SetHeader("Subject", mailMessageItem.Subject)
	obj.mailMessage.SetDateHeader("X-Date", time.Now())
	obj.mailMessage.SetBody("text/html", mailMessageItem.Content)

	err := gomail.Send(obj.mailSender, obj.mailMessage)
	if err != nil {
		Logger.Print(err)
		obj.mailSender = nil
		return
	}

	senderMailOK = true
	Logger.Printf("send mail %v success", mailMessageItem)
}

func (obj *TMailHandler) Sender(to []string, subject string, content string) {
	mailMessageItem := &TMailMessageItem{
		To:to,
		Subject:subject,
		Content:content,
		Time:time.Now(),
		FailNum:0,
	}

	obj.mailMessageItemChannel <- mailMessageItem
}

func (obj *TMailHandler) Stop() {
S:
	obj.stopChannel <- 0

	for {
		n, ok := <-obj.stopChannel
		if !ok {
			break
		}

		if n > 0 {
			break
		} else {
			goto S
		}
	}

	close(obj.stopChannel)
	Logger.Print("mailHandler stopped")
}

var GmailHandler = &TMailHandler{}