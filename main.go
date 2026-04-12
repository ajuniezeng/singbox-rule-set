package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sagernet/sing-box/common/geosite"
	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"

	"github.com/google/go-github/v45/github"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

var githubClient *github.Client

func init() {
	accessToken, loaded := os.LookupEnv("ACCESS_TOKEN")
	if !loaded {
		githubClient = github.NewClient(nil)
		return
	}
	transport := &github.BasicAuthTransport{
		Username: accessToken,
	}
	githubClient = github.NewClient(transport.Client())
}

func fetch(from string) (*github.RepositoryRelease, error) {
	names := strings.SplitN(from, "/", 2)
	latestRelease, _, err := githubClient.Repositories.GetLatestRelease(context.Background(), names[0], names[1])
	if err != nil {
		return nil, err
	}
	return latestRelease, err
}

func get(downloadURL *string) ([]byte, error) {
	log.Info("download ", *downloadURL)
	response, err := http.Get(*downloadURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

func downloadAsset(release *github.RepositoryRelease, assetName string) ([]byte, error) {
	asset := common.Find(release.Assets, func(it *github.ReleaseAsset) bool {
		return *it.Name == assetName
	})
	checksumAsset := common.Find(release.Assets, func(it *github.ReleaseAsset) bool {
		return *it.Name == assetName+".sha256sum"
	})
	if asset == nil {
		return nil, E.New(assetName, " asset not found in upstream release ", release.Name)
	}
	if checksumAsset == nil {
		return nil, E.New(assetName, " checksum asset not found in upstream release ", release.Name)
	}
	data, err := get(asset.BrowserDownloadURL)
	if err != nil {
		return nil, err
	}
	remoteChecksum, err := get(checksumAsset.BrowserDownloadURL)
	if err != nil {
		return nil, err
	}
	checksumFields := strings.Fields(string(remoteChecksum))
	if len(checksumFields) == 0 {
		return nil, E.New(assetName, " checksum asset is empty")
	}
	checksum := sha256.Sum256(data)
	if hex.EncodeToString(checksum[:]) != checksumFields[0] {
		return nil, E.New(assetName, " checksum mismatch")
	}
	return data, nil
}

func parse(vGeositeData []byte) (map[string][]geosite.Item, error) {
	vGeositeList := routercommon.GeoSiteList{}
	err := proto.Unmarshal(vGeositeData, &vGeositeList)
	if err != nil {
		return nil, err
	}
	domainMap := make(map[string][]geosite.Item)
	for _, vGeositeEntry := range vGeositeList.Entry {
		code := strings.ToLower(vGeositeEntry.CountryCode)
		domains := make([]geosite.Item, 0, len(vGeositeEntry.Domain)*2)
		attributes := make(map[string][]*routercommon.Domain)
		for _, domain := range vGeositeEntry.Domain {
			if len(domain.Attribute) > 0 {
				for _, attribute := range domain.Attribute {
					attributes[attribute.Key] = append(attributes[attribute.Key], domain)
				}
			}
			switch domain.Type {
			case routercommon.Domain_Plain:
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomainKeyword,
					Value: domain.Value,
				})
			case routercommon.Domain_Regex:
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomainRegex,
					Value: domain.Value,
				})
			case routercommon.Domain_RootDomain:
				if strings.Contains(domain.Value, ".") {
					domains = append(domains, geosite.Item{
						Type:  geosite.RuleTypeDomain,
						Value: domain.Value,
					})
				}
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomainSuffix,
					Value: "." + domain.Value,
				})
			case routercommon.Domain_Full:
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomain,
					Value: domain.Value,
				})
			}
		}
		domainMap[code] = common.Uniq(domains)
		for attribute, attributeEntries := range attributes {
			attributeDomains := make([]geosite.Item, 0, len(attributeEntries)*2)
			for _, domain := range attributeEntries {
				switch domain.Type {
				case routercommon.Domain_Plain:
					attributeDomains = append(attributeDomains, geosite.Item{
						Type:  geosite.RuleTypeDomainKeyword,
						Value: domain.Value,
					})
				case routercommon.Domain_Regex:
					attributeDomains = append(attributeDomains, geosite.Item{
						Type:  geosite.RuleTypeDomainRegex,
						Value: domain.Value,
					})
				case routercommon.Domain_RootDomain:
					if strings.Contains(domain.Value, ".") {
						attributeDomains = append(attributeDomains, geosite.Item{
							Type:  geosite.RuleTypeDomain,
							Value: domain.Value,
						})
					}
					attributeDomains = append(attributeDomains, geosite.Item{
						Type:  geosite.RuleTypeDomainSuffix,
						Value: "." + domain.Value,
					})
				case routercommon.Domain_Full:
					attributeDomains = append(attributeDomains, geosite.Item{
						Type:  geosite.RuleTypeDomain,
						Value: domain.Value,
					})
				}
			}
			domainMap[code+"@"+attribute] = common.Uniq(attributeDomains)
		}
	}
	return domainMap, nil
}

func parseGeoIP(vGeoIPData []byte) (map[string][]string, error) {
	vGeoIPList := routercommon.GeoIPList{}
	err := proto.Unmarshal(vGeoIPData, &vGeoIPList)
	if err != nil {
		return nil, err
	}
	cidrMap := make(map[string]map[string]struct{})
	for _, vGeoIPEntry := range vGeoIPList.Entry {
		code := strings.ToLower(vGeoIPEntry.CountryCode)
		if code == "" {
			code = strings.ToLower(vGeoIPEntry.Code)
		}
		if code == "" {
			continue
		}
		if cidrMap[code] == nil {
			cidrMap[code] = make(map[string]struct{})
		}
		for _, cidr := range vGeoIPEntry.Cidr {
			addr, ok := netip.AddrFromSlice(cidr.Ip)
			if !ok {
				return nil, E.New("invalid geoip address for code ", code)
			}
			prefix := netip.PrefixFrom(addr.Unmap(), int(cidr.Prefix))
			if !prefix.IsValid() {
				return nil, E.New("invalid geoip prefix for code ", code)
			}
			cidrMap[code][prefix.String()] = struct{}{}
		}
	}
	result := make(map[string][]string, len(cidrMap))
	for code, cidrSet := range cidrMap {
		cidrList := make([]string, 0, len(cidrSet))
		for cidr := range cidrSet {
			cidrList = append(cidrList, cidr)
		}
		sort.Strings(cidrList)
		result[code] = cidrList
	}
	return result, nil
}

type filteredCodePair struct {
	code    string
	badCode string
}

func filterTags(data map[string][]geosite.Item) {
	var codeList []string
	for code := range data {
		codeList = append(codeList, code)
	}
	var badCodeList []filteredCodePair
	var filteredCodeMap []string
	var mergedCodeMap []string
	for _, code := range codeList {
		codeParts := strings.Split(code, "@")
		if len(codeParts) != 2 {
			continue
		}
		leftParts := strings.Split(codeParts[0], "-")
		var lastName string
		if len(leftParts) > 1 {
			lastName = leftParts[len(leftParts)-1]
		}
		if lastName == "" {
			lastName = codeParts[0]
		}
		if lastName == codeParts[1] {
			delete(data, code)
			filteredCodeMap = append(filteredCodeMap, code)
			continue
		}
		if "!"+lastName == codeParts[1] {
			badCodeList = append(badCodeList, filteredCodePair{
				code:    codeParts[0],
				badCode: code,
			})
		} else if lastName == "!"+codeParts[1] {
			badCodeList = append(badCodeList, filteredCodePair{
				code:    codeParts[0],
				badCode: code,
			})
		}
	}
	for _, it := range badCodeList {
		badList := data[it.badCode]
		if badList == nil {
			panic("bad list not found: " + it.badCode)
		}
		delete(data, it.badCode)
		newMap := make(map[geosite.Item]bool)
		for _, item := range data[it.code] {
			newMap[item] = true
		}
		for _, item := range badList {
			delete(newMap, item)
		}
		newList := make([]geosite.Item, 0, len(newMap))
		for item := range newMap {
			newList = append(newList, item)
		}
		data[it.code] = newList
		mergedCodeMap = append(mergedCodeMap, it.badCode)
	}
	sort.Strings(filteredCodeMap)
	sort.Strings(mergedCodeMap)
	os.Stderr.WriteString("filtered " + strings.Join(filteredCodeMap, ",") + "\n")
	os.Stderr.WriteString("merged " + strings.Join(mergedCodeMap, ",") + "\n")
}

func mergeTags(data map[string][]geosite.Item) {
	var codeList []string
	for code := range data {
		codeList = append(codeList, code)
	}
	var cnCodeList []string
	for _, code := range codeList {
		codeParts := strings.Split(code, "@")
		if len(codeParts) != 2 {
			continue
		}
		if codeParts[1] != "cn" {
			continue
		}
		if !strings.HasPrefix(codeParts[0], "category-") {
			continue
		}
		if strings.HasSuffix(codeParts[0], "-cn") || strings.HasSuffix(codeParts[0], "-!cn") {
			continue
		}
		cnCodeList = append(cnCodeList, code)
	}
	for _, code := range codeList {
		if !strings.HasPrefix(code, "category-") {
			continue
		}
		if !strings.HasSuffix(code, "-cn") {
			continue
		}
		if strings.Contains(code, "@") {
			continue
		}
		cnCodeList = append(cnCodeList, code)
	}
	newMap := make(map[geosite.Item]bool)
	for _, item := range data["geolocation-cn"] {
		newMap[item] = true
	}
	for _, code := range cnCodeList {
		for _, item := range data[code] {
			newMap[item] = true
		}
	}
	newList := make([]geosite.Item, 0, len(newMap))
	for item := range newMap {
		newList = append(newList, item)
	}
	data["geolocation-cn"] = newList
	data["cn"] = append(newList, geosite.Item{
		Type:  geosite.RuleTypeDomainSuffix,
		Value: "cn",
	})
	println("merged cn categories: " + strings.Join(cnCodeList, ","))
}

func writeRuleSet(path string, plainRuleSet option.PlainRuleSet, unstable bool) error {
	outputRuleSet, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outputRuleSet.Close()
	return srs.Write(outputRuleSet, plainRuleSet, unstable)
}

func generate(release *github.RepositoryRelease, ruleSetOutput string, ruleSetUnstableOutput string) error {
	geoIPData, err := downloadAsset(release, "geoip.dat")
	if err != nil {
		return err
	}
	geoSiteData, err := downloadAsset(release, "geosite.dat")
	if err != nil {
		return err
	}
	domainMap, err := parse(geoSiteData)
	if err != nil {
		return err
	}
	ipMap, err := parseGeoIP(geoIPData)
	if err != nil {
		return err
	}
	filterTags(domainMap)
	mergeTags(domainMap)
	os.RemoveAll(ruleSetOutput)
	os.RemoveAll(ruleSetUnstableOutput)
	err = os.MkdirAll(ruleSetOutput, 0o755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(ruleSetUnstableOutput, 0o755)
	if err != nil {
		return err
	}
	for code, domains := range domainMap {
		var headlessRule option.DefaultHeadlessRule
		defaultRule := geosite.Compile(domains)
		headlessRule.Domain = defaultRule.Domain
		headlessRule.DomainSuffix = defaultRule.DomainSuffix
		headlessRule.DomainKeyword = defaultRule.DomainKeyword
		headlessRule.DomainRegex = defaultRule.DomainRegex
		var plainRuleSet option.PlainRuleSet
		plainRuleSet.Rules = []option.HeadlessRule{
			{
				Type:           C.RuleTypeDefault,
				DefaultOptions: headlessRule,
			},
		}
		srsPath, _ := filepath.Abs(filepath.Join(ruleSetOutput, "geosite-"+code+".srs"))
		unstableSRSPath, _ := filepath.Abs(filepath.Join(ruleSetUnstableOutput, "geosite-"+code+".srs"))
		err = writeRuleSet(srsPath, plainRuleSet, false)
		if err != nil {
			return err
		}
		err = writeRuleSet(unstableSRSPath, plainRuleSet, true)
		if err != nil {
			return err
		}
	}
	for code, cidrs := range ipMap {
		var headlessRule option.DefaultHeadlessRule
		headlessRule.IPCIDR = cidrs
		var plainRuleSet option.PlainRuleSet
		plainRuleSet.Rules = []option.HeadlessRule{
			{
				Type:           C.RuleTypeDefault,
				DefaultOptions: headlessRule,
			},
		}
		srsPath, _ := filepath.Abs(filepath.Join(ruleSetOutput, "geoip-"+code+".srs"))
		unstableSRSPath, _ := filepath.Abs(filepath.Join(ruleSetUnstableOutput, "geoip-"+code+".srs"))
		err = writeRuleSet(srsPath, plainRuleSet, false)
		if err != nil {
			return err
		}
		err = writeRuleSet(unstableSRSPath, plainRuleSet, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func setActionOutput(name string, content string) {
	os.Stdout.WriteString("::set-output name=" + name + "::" + content + "\n")
}

func release(source string, destination string, ruleSetOutput string, ruleSetOutputUnstable string) error {
	sourceRelease, err := fetch(source)
	if err != nil {
		return err
	}
	destinationRelease, err := fetch(destination)
	if err != nil {
		log.Warn("missing destination latest release")
	} else {
		if os.Getenv("NO_SKIP") != "true" && strings.Contains(*destinationRelease.Name, *sourceRelease.Name) {
			log.Info("already latest")
			setActionOutput("skip", "true")
			return nil
		}
	}
	err = generate(sourceRelease, ruleSetOutput, ruleSetOutputUnstable)
	if err != nil {
		return err
	}
	setActionOutput("tag", *sourceRelease.Name)
	return nil
}

func main() {
	err := release(
		"Loyalsoldier/v2ray-rules-dat",
		"ajuniezeng/singbox-rule-set",
		"rule-set",
		"rule-set-unstable",
	)
	if err != nil {
		log.Fatal(err)
	}
}
