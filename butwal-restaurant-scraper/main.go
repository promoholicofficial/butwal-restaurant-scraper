package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// ─── CONFIG ──────────────────────────────────────────────────────────────────
const (
	maxPerQuery    = 20 // max leads to visit per search query
	scrollAttempts = 8  // more scrolls = more cards loaded
	scrollDelay    = 1800 * time.Millisecond
	pageLoadWait   = 3500 * time.Millisecond
)

var phoneRe = regexp.MustCompile(`\b(98|97)\d{8}\b`)

type lead struct {
	name string
	url  string
}

func main() {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
		SlowMo:   playwright.Float(80),
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{Width: 1280, Height: 900},
		UserAgent: playwright.String(
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
				"AppleWebKit/537.36 (KHTML, like Gecko) " +
				"Chrome/124.0.0.0 Safari/537.36",
		),
	})
	if err != nil {
		log.Fatalf("could not create context: %v", err)
	}
	defer ctx.Close()

	page, err := ctx.NewPage()
	if err != nil {
		log.Fatalf("could not create page: %v", err)
	}

	// ── Jewellery-shop search queries for Butwal ─────────────────────────────
	searchQueries := []string{
		"jewellery shop Butwal",
		"gold shop Butwal",
		"silver ornaments shop Butwal",
		"sunar pasal Butwal",
		"diamond jewellery Butwal",
		"bangles and ornaments shop Butwal",
	}

	results := [][]string{{"Business Name", "Phone", "Category"}}
	globalSeen := make(map[string]bool) // dedup across all queries

	for _, query := range searchQueries {
		fmt.Printf("\n🔎 Searching: %s\n", query)

		// ── 1. Load search results ────────────────────────────────────────
		searchURL := "https://www.google.com/maps/search/" +
			strings.ReplaceAll(query, " ", "+")

		if _, err := page.Goto(searchURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(60000),
		}); err != nil {
			fmt.Printf("  ❌ Failed to load search: %v\n", err)
			continue
		}
		time.Sleep(4 * time.Second)

		// ── 2. Scroll feed to load all cards ─────────────────────────────
		for i := 0; i < scrollAttempts; i++ {
			page.Evaluate(`
				(() => {
					const feed = document.querySelector('div[role="feed"]');
					if (feed) feed.scrollBy(0, 2500);
					else window.scrollBy(0, 2500);
				})()
			`, nil)
			time.Sleep(scrollDelay)
		}
		time.Sleep(1 * time.Second) // final settle

		// ── 3. Extract ALL hrefs+names via JS — plain strings, no handles ─
		raw, err := page.Evaluate(`
			(() => {
				const cards = document.querySelectorAll('a[href*="/maps/place/"]');
				const results = [];
				for (const a of cards) {
					const name = (a.getAttribute('aria-label') || '').trim();
					const href = a.getAttribute('href') || '';
					if (name && href) {
						results.push({ name, href });
					}
				}
				return JSON.stringify(results);
			})()
		`, nil)
		if err != nil {
			fmt.Printf("  ❌ JS extract failed: %v\n", err)
			continue
		}

		jsonStr, ok := raw.(string)
		if !ok || jsonStr == "" || jsonStr == "[]" {
			fmt.Println("  ⚠️  No cards found — Maps layout may have changed")
			continue
		}

		leads := parseLeads(jsonStr, globalSeen, maxPerQuery)
		fmt.Printf("  Harvested %d new leads\n", len(leads))

		if len(leads) == 0 {
			continue
		}

		// ── 4. Visit each place page directly ────────────────────────────
		for i, l := range leads {
			fmt.Printf("  [%d/%d] %s\n", i+1, len(leads), l.name)

			placeURL := l.url
			if !strings.HasPrefix(placeURL, "http") {
				placeURL = "https://www.google.com" + placeURL
			}

			if _, err := page.Goto(placeURL, playwright.PageGotoOptions{
				WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				Timeout:   playwright.Float(30000),
			}); err != nil {
				fmt.Printf("    ❌ Navigation failed: %v\n", err)
				globalSeen[l.name] = true
				continue
			}
			time.Sleep(pageLoadWait)

			// ── 5. Extract phone ──────────────────────────────────────────
			phone := extractPhone(page)
			if phone == "" {
				phone = extractPhoneFromHTML(page)
			}

			globalSeen[l.name] = true

			if phone != "" {
				fmt.Printf("    ✅ %s\n", phone)
				results = append(results, []string{l.name, phone, query})
			} else {
				fmt.Println("    ⚠️  No phone found")
			}

			time.Sleep(300 * time.Millisecond)
		}
	}

	// ── Save CSV ──────────────────────────────────────────────────────────────
	outFile := "butwal_jewellery_leads.csv"
	f, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("could not create CSV: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	w.WriteAll(results)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("🚀 Saved → %s\n", outFile)
	fmt.Printf("📊 Businesses with phones: %d\n", len(results)-1)
	fmt.Println(strings.Repeat("=", 60))
}

// parseLeads parses the JSON string returned from JS and deduplicates.
func parseLeads(jsonStr string, seen map[string]bool, max int) []lead {
	var leads []lead
	nameRe := regexp.MustCompile(`"name"\s*:\s*"([^"]*)"`)
	hrefRe := regexp.MustCompile(`"href"\s*:\s*"([^"]*)"`)

	objRe := regexp.MustCompile(`\{[^}]+\}`)
	objects := objRe.FindAllString(jsonStr, -1)

	for _, obj := range objects {
		if len(leads) >= max {
			break
		}
		nm := nameRe.FindStringSubmatch(obj)
		hr := hrefRe.FindStringSubmatch(obj)
		if len(nm) < 2 || len(hr) < 2 {
			continue
		}
		name := nm[1]
		href := hr[1]
		if name == "" || href == "" || seen[name] {
			continue
		}
		leads = append(leads, lead{name, href})
	}
	return leads
}

// extractPhone tries targeted selectors on the loaded place page.
func extractPhone(page playwright.Page) string {
	// Strategy 1: tel: link — most reliable
	telLink, _ := page.QuerySelector(`a[href^="tel:"]`)
	if telLink != nil {
		href, _ := telLink.GetAttribute("href")
		if p := parsePhone(href); p != "" {
			return p
		}
	}

	// Strategy 2: button with phone encoded in data-item-id
	btn, _ := page.QuerySelector(`button[data-item-id^="phone:tel"]`)
	if btn != nil {
		id, _ := btn.GetAttribute("data-item-id")
		if p := parsePhone(id); p != "" {
			return p
		}
	}

	// Strategy 3: visible text in known phone-container divs
	for _, sel := range []string{
		`div[data-item-id^="phone"]`,
		`div[class*="rogA2c"]`,
		`div[class*="Io6YTe"]`,
		`span[class*="UsdlK"]`,
	} {
		el, _ := page.QuerySelector(sel)
		if el == nil {
			continue
		}
		text, _ := el.TextContent()
		if m := phoneRe.FindString(normalizeDigits(text)); m != "" {
			return m
		}
	}

	return ""
}

// extractPhoneFromHTML scans raw HTML for tel: URIs — last resort.
func extractPhoneFromHTML(page playwright.Page) string {
	html, err := page.Content()
	if err != nil {
		return ""
	}
	telRe := regexp.MustCompile(`tel:([\+\d\s\-]{7,16})`)
	for _, m := range telRe.FindAllStringSubmatch(html, -1) {
		if p := parsePhone(m[1]); p != "" {
			return p
		}
	}
	return ""
}

// parsePhone strips country code and validates a Nepali mobile number.
func parsePhone(raw string) string {
	digitsRe := regexp.MustCompile(`\d+`)
	digits := digitsRe.FindAllString(raw, -1)
	joined := strings.Join(digits, "")
	for _, prefix := range []string{"977", "0"} {
		if strings.HasPrefix(joined, prefix) {
			candidate := joined[len(prefix):]
			if m := phoneRe.FindString(candidate); m != "" {
				return m
			}
		}
	}
	return phoneRe.FindString(joined)
}

// normalizeDigits collapses spaces/dashes between digit pairs only.
func normalizeDigits(s string) string {
	re := regexp.MustCompile(`(\d)[\s\-]+(\d)`)
	for re.MatchString(s) {
		s = re.ReplaceAllString(s, "$1$2")
	}
	return s
}