//go:generate msgp
//msgp:ignore CoaRepository
package coa

import (
	"fmt"
	"sort"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/tinylib/msgp/msgp"
)

type ChartOfAccounts struct {
	Id                      string    `json:"_id"`
	Name                    string    `json:"name"`
	RetainedEarningsAccount string    `json:"retainedEarningsAccount"`
	User                    string    `json:"user"`
	AsOf                    time.Time `json:"timestamp"`
	Created                 time.Time `json:"-"`
	Removed                 time.Time `json:"-"`
}

type Account struct {
	Id      string    `json:"_id"`
	Number  string    `json:"number"`
	Name    string    `json:"name"`
	Tags    Tags      `json:"tags"`
	Parent  string    `json:"parent"`
	User    string    `json:"user"`
	AsOf    time.Time `json:"timestamp"`
	Created time.Time `json:"-"`
	Removed time.Time `json:"-"`
}

type ChartsOfAccounts []*ChartOfAccounts
type Accounts []*Account
type Tags []string

var inheritedProperties = map[string]string{
	"balanceSheet":    "financial statement",
	"incomeStatement": "financial statement",
	"operating":       "income statement attribute",
	"deduction":       "income statement attribute",
	"salesTax":        "income statement attribute",
	"cost":            "income statement attribute",
	"nonOperatingTax": "income statement attribute",
	"incomeTax":       "income statement attribute",
	"dividends":       "income statement attribute",
}

var nonInheritedProperties = map[string]string{
	"increaseOnDebit":  "",
	"increaseOnCredit": "",
	"detail":           "",
	"summary":          "",
}

type KeyValueStore interface {
	Get([]byte) ([]byte, error)
	Put([]byte, []byte) error
}

type CoaRepository struct {
	store KeyValueStore
}

func NewCoaRepository(store KeyValueStore) *CoaRepository {
	return &CoaRepository{store}
}

func (r *CoaRepository) AllChartsOfAccounts() (ChartsOfAccounts, error) {
	var result ChartsOfAccounts
	err := r.get("charts-of-accounts", &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *CoaRepository) GetChartOfAccounts(coaid string) (*ChartOfAccounts, error) {
	coas, err := r.AllChartsOfAccounts()
	if err != nil {
		return nil, err
	}
	for _, coa := range coas {
		if coa.Id == coaid {
			return coa, nil
		}
	}
	return nil, nil
}

func (r *CoaRepository) SaveChartOfAccounts(coa *ChartOfAccounts) (*ChartOfAccounts, error) {
	if coa == nil {
		return nil, fmt.Errorf("Invalid argument: coa is nil")
	}
	if msg := coa.ValidationMessage(); msg != "" {
		return nil, fmt.Errorf(msg)
	}
	coas, err := r.AllChartsOfAccounts()
	if err != nil {
		return nil, err
	}
	coa.AsOf = time.Now()
	if coa.Id == "" {
		coa.Id = uuid.NewV4().String()
		coa.Created = time.Now()
		coas = append(coas, coa)
	} else {
		for i, eachcoa := range coas {
			if eachcoa.Id == coa.Id {
				coas[i] = coa
				break
			}
		}
	}
	sort.Slice(coas, func(i, j int) bool { return strings.Compare(coas[i].Name, coas[j].Name) < 0 })
	err = r.put("charts-of-accounts", coas)
	if err != nil {
		return nil, err
	}
	return coa, nil
}

func (r *CoaRepository) AllAccounts(coaid string) (Accounts, error) {
	if coaid == "" {
		return nil, fmt.Errorf("Invalid argument: coaid is empty")
	}
	var result Accounts
	err := r.get("accounts/"+coaid, &result)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return strings.Compare(result[i].Number, result[j].Number) < 0 })
	return result, nil
}

func (r *CoaRepository) GetAccount(coaid string, id string) (*Account, error) {
	aa, err := r.AllAccounts(coaid)
	if err != nil {
		return nil, err
	}
	for _, a := range aa {
		if a.Id == id {
			return a, nil
		}
	}
	return nil, nil
}

func (r *CoaRepository) SaveAccount(coaid string, account *Account) (*Account, error) {
	if coaid == "" {
		return nil, fmt.Errorf("Invalid argument: coaid is empty")
	}
	if account == nil {
		return nil, fmt.Errorf("Invalid argument: account is nil")
	}
	var tags []string
	var retainedEarningsAccount bool
	for _, k := range account.Tags {
		if k == "retainedEarnings" {
			retainedEarningsAccount = true
		}
		_, ok1 := inheritedProperties[k]
		_, ok2 := nonInheritedProperties[k]
		if ok1 || ok2 {
			tags = append(tags, k)
		}
	}
	if !account.Tags.Contains("detail") && account.Id == "" {
		tags = append(tags, "detail")
	}
	account.Tags = tags
	account.AsOf = time.Now()
	if account.Id != "" {
		old, err := r.GetAccount(coaid, account.Id)
		if err != nil {
			return nil, err
		}
		account.Number = old.Number
		account.Parent = old.Parent
		account.Created = old.Created
	}
	if msg := account.ValidationMessage(coaid, r); msg != "" {
		return nil, fmt.Errorf(msg)
	}
	var accounts Accounts
	err := r.get("accounts/"+coaid, &accounts)
	if err != nil {
		return nil, err
	}
	if account.Id == "" {
		account.Id = uuid.NewV4().String()
		account.Created = time.Now()
		accounts = append(accounts, account)
	} else {
		for i, a := range accounts {
			if account.Id == a.Id {
				accounts[i] = account
				break
			}
		}
	}
	err = r.put("accounts/"+coaid, accounts)
	if err != nil {
		return nil, err
	}
	if retainedEarningsAccount {
		coa, err := r.GetChartOfAccounts(coaid)
		if err != nil {
			return nil, err
		}
		coa.RetainedEarningsAccount = account.Id
		_, err = r.SaveChartOfAccounts(coa)
		if err != nil {
			return nil, err
		}
	}
	if account.Parent != "" {
		parent, err := r.GetAccount(coaid, account.Parent)
		if err != nil {
			return nil, err
		}
		changed := false
		i := parent.Tags.IndexOf("detail")
		if i != -1 {
			parent.Tags = append(parent.Tags[:i], parent.Tags[i+1:]...)
			changed = true
		}
		if !parent.Tags.Contains("summary") {
			parent.Tags = append(parent.Tags, "summary")
			changed = true
		}
		if changed {
			_, err := r.SaveAccount(coaid, parent)
			if err != nil {
				return nil, err
			}
		}
	}
	return account, nil
}

func (r *CoaRepository) Indexes(coaid string, accountsIds []string, tags []string) ([]int, error) {
	if coaid == "" {
		return nil, fmt.Errorf("Invalid argument: coaid is empty")
	}
	var accounts Accounts
	err := r.get("accounts/"+coaid, &accounts)
	if err != nil {
		return nil, err
	}
	result := make([]int, len(accountsIds))
	for i, id := range accountsIds {
		result[i] = -1
		for j, a := range accounts {
			if a.Id == id && a.Tags.ContainsAll(tags) {
				result[i] = j
			}
		}
	}
	return result, nil
}

// TODO: DeleteAccount

func (coa *ChartOfAccounts) ValidationMessage() string {
	if len(strings.TrimSpace(coa.Name)) == 0 {
		return "The name must be informed"
	}
	return ""
}

func (account *Account) ValidationMessage(coaid string, r *CoaRepository) string {
	if len(strings.TrimSpace(account.Number)) == 0 {
		return "The number must be informed"
	}
	if len(strings.TrimSpace(account.Name)) == 0 {
		return "The name must be informed"
	}
	if !account.Tags.Contains("balanceSheet") && !account.Tags.Contains("incomeStatement") {
		return "The financial statement must be informed"
	}
	if account.Tags.Contains("balanceSheet") && account.Tags.Contains("incomeStatement") {
		return "The statement must be either balance sheet or income statement"
	}
	if !account.Tags.Contains("increaseOnDebit") && !account.Tags.Contains("increaseOnCredit") {
		return "The normal balance must be informed"
	}
	if account.Tags.Contains("increaseOnDebit") && account.Tags.Contains("increaseOnCredit") {
		return "The normal balance must be either debit or credit"
	}
	count := 0
	for _, p := range account.Tags {
		if inheritedProperties[p] == "income statement attribute" {
			count++
		}
	}
	if count > 1 {
		return "Only one income statement attribute is allowed"
	}
	if account.Id == "" {
		aa, err := r.AllAccounts(coaid)
		if err != nil {
			return err.Error()
		}
		for _, a := range aa {
			if a.Number == account.Number {
				return "An account with this number already exists"
			}
		}
	}
	if account.Parent != "" {
		parent, err := r.GetAccount(coaid, account.Parent)
		if err != nil {
			return err.Error()
		}
		if parent == nil {
			return "Parent not found: " + account.Parent
		}
		if !strings.HasPrefix(account.Number, parent.Number) {
			return "The number must start with parent's number"
		}
		for key, value := range inheritedProperties {
			if parent.Tags.Contains(key) && !account.Tags.Contains(key) {
				return "The " + value + " must be same as the parent"
			}
		}
	}
	return ""
}

func (r *CoaRepository) put(key string, v interface{}) error {
	// data, err := json.Marshal(v)
	data, err := v.(msgp.Marshaler).MarshalMsg(nil)
	if err != nil {
		return err
	}
	return r.store.Put([]byte(key), data)
}

func (r *CoaRepository) get(key string, v interface{}) error {
	data, err := r.store.Get([]byte(key))
	if err != nil {
		return err
	}
	if data == nil || len(data) == 0 {
		return nil
	}
	_, err = v.(msgp.Unmarshaler).UnmarshalMsg(data)
	// err = json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func (aa Accounts) String() string {
	ss := make([]string, len(aa))
	for i, a := range aa {
		ss[i] = a.String()
	}
	return strings.Join(ss, ", ")
}

func (a *Account) String() string {
	return fmt.Sprint(*a)
}

func (c Tags) IndexOf(s string) int {
	for i, each := range c {
		if each == s {
			return i
		}
	}
	return -1
}

func (c Tags) Contains(s string) bool { return c.IndexOf(s) != -1 }

func (c Tags) ContainsAll(ss []string) bool {
	for _, s := range ss {
		if !c.Contains(s) {
			return false
		}
	}
	return true
}
