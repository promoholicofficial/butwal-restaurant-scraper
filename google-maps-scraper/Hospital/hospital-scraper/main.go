package main

import (
    "encoding/csv"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    "github.com/gocolly/colly/v2"
)

type Hospital struct {
    Name    string
    Address string
    Phone   string
    Type    string
}

func main() {
    fmt.Println("🏥 Starting Hospital Scraper for Butwal Area...")
    
    hospitals := []Hospital{}
    
    // Create a new collector
    c := colly.NewCollector(
        colly.AllowedDomains("justdial.com", "www.justdial.com"),
        colly.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"),
    )

    // Set timeout
    c.SetRequestTimeout(120 * time.Second)

    // Before making a request
    c.OnRequest(func(r *colly.Request) {
        fmt.Println("Visiting:", r.URL.String())
    })

    // On every HTML element
    c.OnHTML(".jsx-2137425665", func(e *colly.HTMLElement) {
        name := strings.TrimSpace(e.ChildText(".jsx-1088245243"))
        address := strings.TrimSpace(e.ChildText(".jsx-1088245243 .jsx-2652900348"))
        phone := strings.TrimSpace(e.ChildText(".jsx-754050147"))

        if name != "" {
            hospital := Hospital{
                Name:    name,
                Address: address,
                Phone:   phone,
                Type:    "Hospital",
            }
            hospitals = append(hospitals, hospital)
            fmt.Printf("✅ Found: %s\n", name)
        }
    })

    // Error handling
    c.OnError(func(r *colly.Response, err error) {
        fmt.Println("❌ Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
    })

    // Start scraping
    searchURL := "https://www.justdial.com/Butwal/Hospitals/nct-10117767"
    c.Visit(searchURL)

    // Wait for completion
    c.Wait()

    // Save to CSV
    if len(hospitals) > 0 {
        saveToCSV(hospitals)
        fmt.Printf("\n✅ Successfully scraped %d hospitals!\n", len(hospitals))
        fmt.Println("📄 Data saved to: hospitals_butwal.csv")
    } else {
        fmt.Println("\n⚠️  No hospitals found. Trying alternative method...")
        scrapeGoogleMaps()
    }
}

func saveToCSV(hospitals []Hospital) {
    file, err := os.Create("hospitals_butwal.csv")
    if err != nil {
        log.Fatal("❌ Cannot create file:", err)
    }
    defer file.Close()

    writer := csv.NewWriter(file)
    defer writer.Flush()

    // Write header
    writer.Write([]string{"Name", "Address", "Phone", "Type"})

    // Write data
    for _, h := range hospitals {
        writer.Write([]string{h.Name, h.Address, h.Phone, h.Type})
    }
}

func scrapeGoogleMaps() {
    fmt.Println("\n📝 Manual collection recommended for Google Maps")
    fmt.Println("Visit: https://www.google.com/maps/search/hospitals+near+Butwal")
}
