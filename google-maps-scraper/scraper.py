from playwright.sync_api import sync_playwright
import pandas as pd
import time

def scrape_google_maps(query, max_results=50):
    results = []

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=False)
        page = browser.new_page()

        # Go to Google Maps with your query
        search_url = f"https://www.google.com/maps/search/{query.replace(' ', '+')}"
        page.goto(search_url)
        time.sleep(3)

        # Scroll to load more results
        scrollable_div = page.locator('div[role="feed"]')
        for _ in range(10):
            scrollable_div.evaluate("el => el.scrollBy(0, 1000)")
            time.sleep(1)

        # Get all listing elements
        listings = page.locator('a[href*="/maps/place/"]').all()

        for listing in listings[:max_results]:
            try:
                listing.click()
                time.sleep(2)

                name = page.locator('h1').first.inner_text()
                
                try:
                    phone = page.locator('[data-item-id*="phone"]').first.inner_text()
                except:
                    phone = "N/A"

                try:
                    address = page.locator('[data-item-id="address"]').first.inner_text()
                except:
                    address = "N/A"

                try:
                    website = page.locator('a[data-item-id="authority"]').first.get_attribute("href")
                except:
                    website = "N/A"

                try:
                    rating = page.locator('div.F7nice span').first.inner_text()
                except:
                    rating = "N/A"

                results.append({
                    "Name": name,
                    "Phone": phone,
                    "Address": address,
                    "Website": website,
                    "Rating": rating
                })

                print(f"✅ Scraped: {name}")

            except Exception as e:
                print(f"⚠️ Error: {e}")
                continue

        browser.close()

    return results

# ---- RUN IT ----
query = "restaurants in Butwal Nepal"   # ← Change this to whatever you want
data = scrape_google_maps(query, max_results=50)

df = pd.DataFrame(data)
df.to_csv("butwal_businesses.csv", index=False)
print(f"\n✅ Done! {len(df)} businesses saved to butwal_businesses.csv")
