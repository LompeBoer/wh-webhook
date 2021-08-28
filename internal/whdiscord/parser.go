package whdiscord

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Parser struct {
	regexTitle         *regexp.Regexp
	regexMessageNumber *regexp.Regexp
	regexOpen          *regexp.Regexp
	regexClose         *regexp.Regexp
	regexDCA           *regexp.Regexp
	Style              string
}

func NewParser(style string) *Parser {
	return &Parser{
		regexTitle:         regexp.MustCompile(`'([0-9]+)' ([A-Z]\w+) #([0-9]+) ([A-Z]\w+)`),
		regexMessageNumber: regexp.MustCompile(`'([0-9]+)'.*`),
		regexOpen:          regexp.MustCompile(`(?s)Pair: ([A-Z]\w+)(.*?)Direction: ([A-Z]\w+)`),
		regexClose:         regexp.MustCompile(`(?s)Pair: ([A-Z]\w+).*?Direction: ([A-Z]\w+).*?Profit: ([0-9.,]+).*?Number of Buys: ([0-9]+)`),
		regexDCA:           regexp.MustCompile(`(?s)Pair: ([A-Z]\w+).*?Direction: ([A-Z]\w+).*?Number of Buys: ([0-9]+)`),
		Style:              style,
	}
}

type DiscordWebhook struct {
	Content string         `json:"content"`
	Embeds  []DiscordEmbed `json:"embeds"`
}

type DiscordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	URL         string              `json:"url"`
	Timestamp   string              `json:"timestamp"`
	Color       int                 `json:"color"`
	Fields      []DiscordEmbedField `json:"fields"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func (p *Parser) UnmarshalDiscordMessage(m []byte) (DiscordWebhook, error) {
	var dwh DiscordWebhook
	err := json.Unmarshal(m, &dwh)
	if err != nil {
		return dwh, err
	}
	return dwh, nil
}

type OriginalData struct {
	Color          int
	MessageNumber  int
	PositionNumber int
	Type           string
	Pair           string
	Direction      string
	NumberOfBuys   int
	Profit         string
}

func (p *Parser) DecodeOriginalMessage(original DiscordWebhook) (OriginalData, error) {
	data := OriginalData{}

	var embed DiscordEmbed
	var title string
	if original.Embeds == nil || len(original.Embeds) == 0 {
		title = original.Content
	} else {
		embed = original.Embeds[0]
		title = embed.Title
	}

	err := p.ParseTitle(title, &data)
	if err != nil {
		return data, err
	}
	err = p.ParseDescription(embed.Description, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func (p *Parser) ParseTitle(title string, data *OriginalData) error {
	rt := regexp.MustCompile(`'([0-9]+)' ([A-Z]\w+) #([0-9]+) ([A-Z]\w+)`)
	if rt.MatchString(title) {

		match := rt.FindAllStringSubmatch(title, 1)
		if match == nil {
			return errors.New("no title submatch")
		}

		msgnr, err := strconv.Atoi(match[0][1])
		if err != nil {
			return errors.New("numbers not found")
		}
		data.MessageNumber = msgnr

		posnr, err := strconv.Atoi(match[0][3])
		if err != nil {
			return errors.New("numbers not found")
		}
		data.PositionNumber = posnr

		orderType := match[0][4]
		buyType := match[0][2]

		if orderType == "Opened" {
			if buyType == "Position" {
				data.Type = "open"
			} else if buyType == "DCA" {
				data.Type = "dca"
			}
		} else if orderType == "Closed" {
			data.Type = "close"
		} else {
			return errors.New("unknown embed title")
		}
	} else {
		rn := regexp.MustCompile(`'([0-9]+)'.*`)
		m := rn.FindAllStringSubmatch(title, 1)
		if m == nil {
			return errors.New("no plain title match")
		}
		data.MessageNumber, _ = strconv.Atoi(m[0][1])

		if strings.Contains(title, "Skipped: Isolation Mode") {
			data.Type = "isolation"
		} else if strings.Contains(title, "Bot Started") {
			data.Type = "start"
		} else if strings.Contains(title, "Bot Stopped") {
			data.Type = "stop"
		} else {
			return errors.New("unknown content title")
		}
	}

	return nil
}

func (p *Parser) ParseDescription(desc string, data *OriginalData) error {
	if data.Type == "open" {
		re4 := regexp.MustCompile(`(?s)Pair: ([A-Z]\w+)(.*?)Direction: ([A-Z]\w+)`)
		match4 := re4.FindAllStringSubmatch(desc, 1)
		if match4 == nil {
			return errors.New("open desription invalid")
		}

		data.Pair = match4[0][1]
		data.Direction = match4[0][3]
		data.NumberOfBuys = 1
	} else if data.Type == "close" {
		re5 := regexp.MustCompile(`(?s)Pair: ([A-Z]\w+).*?Direction: ([A-Z]\w+).*?Profit: ([0-9.,]+).*?Number of Buys: ([0-9]+)`)
		match5 := re5.FindAllStringSubmatch(desc, 1)
		if match5 == nil {
			return errors.New("close description invalid")
		}

		data.Pair = match5[0][1]
		data.Direction = match5[0][2]
		data.Profit = match5[0][3]
		data.NumberOfBuys, _ = strconv.Atoi(match5[0][4])
	} else if data.Type == "dca" {
		re6 := regexp.MustCompile(`(?s)Pair: ([A-Z]\w+).*?Direction: ([A-Z]\w+).*?Number of Buys: ([0-9]+)`)
		match6 := re6.FindAllStringSubmatch(desc, 1)
		if match6 == nil {
			return errors.New("dca description invalid")
		}

		data.Pair = match6[0][1]
		data.Direction = match6[0][2]
		data.NumberOfBuys, _ = strconv.Atoi(match6[0][3])
	}

	return nil
}

const (
	KeywordPair           = "${PAIR}"
	KeywordDirection      = "${DIRECTION}"
	KeywordPositionNumber = "${POSITION_NUMBER}"
	KeywordMessageNumber  = "${MESSAGE_NUMBER}"
	KeywordProfit         = "${PROFIT}"
	KeywordNumberOfBuys   = "${NUMBER_OF_BUYS}"
	KeywordType           = "${TYPE}"
	KeywordColor          = "${COLOR}"
)

func (p *Parser) ReplaceKeywords(data OriginalData, style string) string {
	var filename string
	switch data.Type {
	case "open":
		filename = "open.json"
	case "dca":
		filename = "dca.json"
	case "close":
		filename = "close.json"
	case "isolation":
		filename = "isolation.json"
	case "start":
		filename = "start.json"
	case "stop":
		filename = "stop.json"
	}
	jsonString := p.ReadJSONFile(filename, style)

	r := strings.NewReplacer(
		KeywordPair, data.Pair,
		KeywordDirection, data.Direction,
		KeywordPositionNumber, strconv.Itoa(data.PositionNumber),
		KeywordMessageNumber, strconv.Itoa(data.MessageNumber),
		KeywordProfit, data.Profit,
		KeywordNumberOfBuys, strconv.Itoa(data.NumberOfBuys),
		KeywordType, data.Type,
		KeywordColor, strconv.Itoa(data.Color),
	)

	return r.Replace(jsonString)
}

func (p *Parser) ReadJSONFile(filename string, style string) string {
	b, err := os.ReadFile("messages/" + style + "/" + filename)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func (p *Parser) ParseMessage(m []byte) []byte {
	original, err := p.UnmarshalDiscordMessage(m)
	if err != nil {
		return m
	}
	data, err := p.DecodeOriginalMessage(original)
	if err != nil {
		log.Println(err.Error())
		return m
	}

	newmsg := p.ReplaceKeywords(data, p.Style)
	return []byte(newmsg)
}
