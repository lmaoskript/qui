package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	statusSucceeded uint8 = iota
	statusRefunded
	statusFailed
	statusDisputed
	statusPending
)

var statusNames = []string{"Succeeded", "Refunded", "Failed", "Disputed", "Pending"}

var methods = []string{"Visa", "Mastercard", "Amex", "SEPA debit", "ACH debit"}

var plans = []struct {
	name  string
	cents int32
}{{"Hobby", 1900}, {"Pro", 4900}, {"Team", 24900}, {"Enterprise", 149900}}

type prng uint64

func (r *prng) next() uint64 {
	*r += 0x9e3779b97f4a7c15
	z := uint64(*r)
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *prng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

func (r *prng) intn(n int) int { return int(r.next() % uint64(n)) }

type customer struct {
	name, email string
	search      string
	created     int
	spent       int64
	payments    int32
	plan        int
	mrr         int32
	active      bool
}

type dayStats struct {
	gross, refunds, fees float64
	succeeded, failed    float64
	signups              float64
}

type movement struct {
	label                               string
	added, expansion, contraction, lost float64
	total                               float64
}

type invoiceStatus uint8

const (
	invoicePaid invoiceStatus = iota
	invoiceOpen
	invoicePastDue
	invoiceDraft
)

var invoiceNames = []string{"Paid", "Open", "Past due", "Draft"}

type invoice struct {
	number   string
	customer int32
	amount   int32
	status   invoiceStatus
	issued   time.Time
	plan     int
	usage    int32
}

type dataset struct {
	asOf      time.Time
	customers []customer
	amount    []int32
	status    []uint8
	buyer     []int32
	when      []int64
	method    []uint8
	days      []dayStats
	today     [24]float64
	yesterday [24]float64
	counts    [5]int
	months    []movement
	invoices  []invoice
	subs      []int32
	active    int
}

const dayCount = 730

var first = []string{"Hana", "Mateo", "Priya", "Tomas", "Lena", "Kofi", "Ines", "Yuki", "Omar", "Freya", "Dario", "Nadia", "Elias", "Mina", "Jonas", "Zara", "Ravi", "Alma", "Theo", "Sana"}
var last = []string{"Okafor", "Lindqvist", "Marchetti", "Tanaka", "Haddad", "Novak", "Silva", "Bergstrom", "Mbeki", "Duarte", "Kowalski", "Reyes", "Aydin", "Moreau", "Ivanov", "Nakamura", "Fischer", "Rahman", "Costa", "Vance"}
var domains = []string{"brightloop", "northkiln", "copperfield", "lumenworks", "tidepool", "fernhollow", "quillstack", "emberline", "saltmarsh", "orbitwell"}

func buildData() *dataset {
	r := prng(20261109)
	now := time.Now()
	d := &dataset{asOf: time.Date(now.Year(), now.Month(), now.Day(), 17, 20, 0, 0, now.Location())}
	d.days = make([]dayStats, dayCount)

	const customerCount = 12000
	d.customers = make([]customer, customerCount)
	for i := range d.customers {
		c := &d.customers[i]
		f, l := first[r.intn(len(first))], last[r.intn(len(last))]
		c.name = f + " " + l
		c.email = strings.ToLower(f+"."+l) + fmt.Sprintf("%d@%s.example", i%97, domains[r.intn(len(domains))])
		c.search = strings.ToLower(c.name + " " + c.email)
		c.created = int(math.Pow(r.float(), 1.5) * (dayCount - 1))
		roll := r.float()
		switch {
		case roll < 0.52:
			c.plan = 0
		case roll < 0.86:
			c.plan = 1
		case roll < 0.98:
			c.plan = 2
		default:
			c.plan = 3
		}
		c.active = r.float() < 0.82
		c.mrr = plans[c.plan].cents
		if c.active {
			d.subs = append(d.subs, int32(i))
			d.active += int(c.mrr)
		}
		d.days[c.created].signups++
	}

	seconds := make([]int, 0, 1024)
	asOfSecond := 17*3600 + 20*60
	for day := 0; day < dayCount; day++ {
		weekday := int(d.asOf.AddDate(0, 0, -day).Weekday())
		weekly := 1.0
		if weekday == 0 || weekday == 6 {
			weekly = 0.72
		}
		trend := 90 + 300*math.Exp(-float64(day)/320)
		count := int(trend * weekly * (0.86 + 0.28*r.float()))
		seconds = seconds[:0]
		for j := 0; j < count; j++ {
			hour := r.float() * 24
			if r.float() < 0.72 {
				hour = 15 + (r.float()+r.float()+r.float()-1.5)*7
			}
			second := int(math.Mod(hour+24, 24) * 3600)
			if day == 0 && second > asOfSecond {
				continue
			}
			seconds = append(seconds, second)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(seconds)))
		midnight := d.asOf.AddDate(0, 0, -day)
		base := time.Date(midnight.Year(), midnight.Month(), midnight.Day(), 0, 0, 0, 0, midnight.Location()).Unix()
		stats := &d.days[day]
		for _, second := range seconds {
			buyer := r.intn(customerCount)
			c := &d.customers[buyer]
			amount := plans[c.plan].cents
			if r.float() < 0.35 {
				amount += int32(r.float() * float64(amount) * 0.6)
			}
			roll := r.float()
			status := statusSucceeded
			switch {
			case roll < 0.86:
			case roll < 0.91:
				status = statusRefunded
			case roll < 0.975:
				status = statusFailed
			case roll < 0.982:
				status = statusDisputed
			default:
				status = statusPending
			}
			if status != statusPending || day < 3 {
				d.amount = append(d.amount, amount)
			} else {
				status = statusSucceeded
				d.amount = append(d.amount, amount)
			}
			d.status = append(d.status, status)
			d.buyer = append(d.buyer, int32(buyer))
			d.when = append(d.when, base+int64(second))
			d.method = append(d.method, uint8(r.intn(len(methods))))
			d.counts[status]++
			value := float64(amount) / 100
			if status != statusFailed && status != statusPending {
				stats.gross += value
				stats.fees += value*0.029 + 0.30
				stats.succeeded++
				c.spent += int64(amount)
				c.payments++
				hour := second / 3600
				if day == 0 {
					d.today[hour] += value
				} else if day == 1 {
					d.yesterday[hour] += value
				}
				if status == statusRefunded {
					stats.refunds += value
				}
			} else if status == statusFailed {
				stats.failed++
			}
		}
	}

	for i := 1; i < 24; i++ {
		d.today[i] += d.today[i-1]
		d.yesterday[i] += d.yesterday[i-1]
	}

	const monthCount = 24
	raw := make([]movement, monthCount)
	total := 40000.0
	for i := range raw {
		m := &raw[i]
		m.added = (3200 + 4200*float64(i)/monthCount) * (0.8 + 0.5*r.float())
		m.expansion = (900 + 2100*float64(i)/monthCount) * (0.7 + 0.6*r.float())
		m.contraction = (420 + 700*float64(i)/monthCount) * (0.7 + 0.6*r.float())
		m.lost = (900 + 1500*float64(i)/monthCount) * (0.7 + 0.6*r.float())
		total += m.added + m.expansion - m.contraction - m.lost
		m.total = total
	}
	scale := float64(d.active) / 100 / total
	d.months = raw
	for i := range d.months {
		m := &d.months[i]
		m.added *= scale
		m.expansion *= scale
		m.contraction *= scale
		m.lost *= scale
		m.total *= scale
		m.label = d.asOf.AddDate(0, i-monthCount+1, 0).Format("Jan 06")
	}

	d.invoices = make([]invoice, 8000)
	for i := range d.invoices {
		inv := &d.invoices[i]
		buyer := r.intn(customerCount)
		inv.customer = int32(buyer)
		c := &d.customers[buyer]
		inv.plan = c.plan
		inv.usage = int32(r.float() * r.float() * float64(plans[c.plan].cents))
		inv.amount = plans[c.plan].cents + inv.usage
		inv.number = fmt.Sprintf("INV-%05d", 48211-i)
		inv.issued = d.asOf.AddDate(0, 0, -i/16)
		roll := r.float()
		switch {
		case i < 12 && roll < 0.5:
			inv.status = invoiceDraft
		case i < 240 && roll < 0.55:
			inv.status = invoiceOpen
		case i < 700 && roll < 0.10:
			inv.status = invoicePastDue
		default:
			inv.status = invoicePaid
		}
	}
	return d
}

func paymentID(i int) string {
	r := prng(uint64(i)*2654435761 + 77)
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var b strings.Builder
	b.WriteString("py_")
	for j := 0; j < 14; j++ {
		b.WriteByte(alphabet[r.next()%uint64(len(alphabet))])
	}
	return b.String()
}

func cardLast4(i int) string {
	r := prng(uint64(i)*40503 + 13)
	return fmt.Sprintf("%04d", r.next()%10000)
}
