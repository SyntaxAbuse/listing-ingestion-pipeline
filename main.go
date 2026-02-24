package main // Declare the main package (entrypoint).

import ( // Begin imports.
	"bytes"          // Used to build HTTP request bodies.
	"context"        // Used for request scoping/timeouts/cancellation.
	"encoding/json"  // Used for JSON encode/decode.
	"errors"         // Used for creating errors.
	"fmt"            // Used for formatting strings and printing.
	"io"             // Used for io.ReadAll (replaces deprecated ioutil).
	"log"            // Used for logging.
	"os"             // Used for environment variable access.
	"regexp"         // Used for extracting numeric price values.
	"strconv"        // Used for converting strings to numbers.
	"strings"        // Used for trimming and replacing text.
	"time"           // Used for timeouts and polite delays.

	"github.com/PuerkitoBio/goquery" // Used to query HTML with jQuery-like selectors.
	"github.com/gocolly/colly/v2"   // Used for scraping with callbacks and request control.
) // End imports.

type AppConfig struct { // Define application configuration loaded from env vars.
	ShopifyShopSubdomain string  // Shopify shop subdomain (e.g., "myshop" for myshop.myshopify.com).
	ShopifyAdminToken    string  // Shopify Admin API access token.
	TargetCollectionID   int64   // Shopify collection ID to add the product to.
	SourceListingURL     string  // Source page URL to extract listing data from.
	PriceMarkupUSD       float64 // Amount to add to scraped price (profit/markup).
	ProductTagsCSV       string  // Comma-separated tags for the product.
	ProductType          string  // Shopify product_type field.
	ProductVendor        string  // Shopify vendor field.
	UserAgent            string  // User-Agent string for scraper politeness/consistency.
} // End AppConfig.

type Listing struct { // Define the extracted listing data we care about.
	Title       string   // Listing title (human readable).
	PriceUSD    float64  // Parsed numeric price (USD or numeric portion).
	Description string   // Listing short description (best-effort).
	ImageURLs   []string // List of image URLs.
} // End Listing.

type ShopifyProductCreateRequest struct { // Define the request payload for Shopify product creation.
	Product ShopifyProduct `json:"product"` // Wrap product in "product" key per Shopify Admin API.
} // End ShopifyProductCreateRequest.

type ShopifyProduct struct { // Define a Shopify product object.
	Title       string                  `json:"title"`        // Product title.
	Vendor      string                  `json:"vendor"`       // Product vendor.
	ProductType string                  `json:"product_type"` // Product type.
	Tags        string                  `json:"tags"`         // Tags as comma-separated string.
	Variants    []ShopifyVariant        `json:"variants"`     // Variants list (at least one).
	Images      []ShopifyProductImage   `json:"images"`       // Images list.
} // End ShopifyProduct.

type ShopifyVariant struct { // Define a Shopify variant object (simplified).
	Price string `json:"price"` // Variant price string (Shopify expects string).
} // End ShopifyVariant.

type ShopifyProductImage struct { // Define Shopify image payload.
	Src string `json:"src"` // Image URL.
} // End ShopifyProductImage.

type ShopifyProductCreateResponse struct { // Define response shape for create product call.
	Product struct { // Product wrapper in response.
		ID int64 `json:"id"` // Newly created product ID.
	} `json:"product"` // Response "product" key.
} // End ShopifyProductCreateResponse.

type ShopifyCollectCreateRequest struct { // Define request payload for adding product to collection.
	Collect ShopifyCollect `json:"collect"` // Wrap collect in "collect" key.
} // End ShopifyCollectCreateRequest.

type ShopifyCollect struct { // Define collect object.
	ProductID    int64 `json:"product_id"`    // Product ID to add.
	CollectionID int64 `json:"collection_id"` // Collection ID to add into.
} // End ShopifyCollect.

type ShopifyClient struct { // Define a small Shopify API client.
	httpClient *http.Client // Underlying HTTP client.
	config     AppConfig    // Configuration for Shopify calls.
} // End ShopifyClient.

func main() { // Program entry.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds) // Set log format to include timestamps.

	cfg, err := loadConfigFromEnv() // Load configuration from environment.
	if err != nil {                // If configuration invalid/missing.
		log.Fatal(err) // Exit and print the error.
	} // End config validation.

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second) // Create an overall timeout context.
	defer cancel()                                                          // Ensure context is canceled on exit.

	listing, err := scrapeListing(ctx, cfg) // Extract listing data from the source URL.
	if err != nil {                          // If scraping fails.
		log.Fatal(err) // Exit with error.
	} // End scrape error handling.

	log.Printf("Extracted listing: title=%q price=%.2f images=%d\n", listing.Title, listing.PriceUSD, len(listing.ImageURLs)) // Log extraction summary.

	shopify := newShopifyClient(cfg) // Build Shopify client.

	productID, err := shopify.CreateProduct(ctx, listing) // Create a Shopify product from listing.
	if err != nil {                                        // If create fails.
		log.Fatal(err) // Exit with error.
	} // End create error handling.

	log.Printf("Created Shopify product: id=%d\n", productID) // Log product ID.

	if cfg.TargetCollectionID != 0 { // Only add to collection if configured.
		if err := shopify.AddProductToCollection(ctx, productID, cfg.TargetCollectionID); err != nil { // Add product to collection.
			log.Fatal(err) // Exit with error.
		} // End add-to-collection error handling.
		log.Printf("Added product %d to collection %d\n", productID, cfg.TargetCollectionID) // Log success.
	} // End optional collection add.
} // End main.

func loadConfigFromEnv() (AppConfig, error) { // Load config from env vars.
	get := func(key string) string { // Small helper to read env var.
		return strings.TrimSpace(os.Getenv(key)) // Return trimmed env var value.
	} // End helper.

	cfg := AppConfig{} // Initialize empty config.

	cfg.ShopifyShopSubdomain = get("SHOPIFY_SHOP") // Read Shopify shop subdomain.
	cfg.ShopifyAdminToken = get("SHOPIFY_TOKEN")   // Read Shopify token.
	cfg.SourceListingURL = get("SOURCE_URL")       // Read source listing URL.

	cfg.ProductVendor = get("PRODUCT_VENDOR") // Read vendor (optional).
	if cfg.ProductVendor == "" {              // If missing.
		cfg.ProductVendor = "Vendor" // Set a safe default.
	} // End default vendor.

	cfg.ProductType = get("PRODUCT_TYPE") // Read product type (optional).
	if cfg.ProductType == "" {            // If missing.
		cfg.ProductType = "Product" // Set default.
	} // End default product type.

	cfg.ProductTagsCSV = get("PRODUCT_TAGS") // Read tags (optional).
	if cfg.ProductTagsCSV == "" {            // If missing.
		cfg.ProductTagsCSV = "tag" // Default tag.
	} // End default tags.

	cfg.UserAgent = get("USER_AGENT") // Read UA (optional).
	if cfg.UserAgent == "" {          // If missing.
		cfg.UserAgent = "Mozilla/5.0 (compatible; ETL-Demo/1.0; +https://example.com)" // Use a polite default UA.
	} // End default UA.

	if markupStr := get("PRICE_MARKUP_USD"); markupStr != "" { // If markup was provided.
		val, err := strconv.ParseFloat(markupStr, 64) // Parse to float.
		if err != nil {                               // If invalid.
			return AppConfig{}, fmt.Errorf("PRICE_MARKUP_USD must be a number: %w", err) // Return error.
		} // End parse error.
		cfg.PriceMarkupUSD = val // Store markup.
	} else { // If no markup.
		cfg.PriceMarkupUSD = 0 // Default markup.
	} // End markup default.

	if collectionStr := get("SHOPIFY_COLLECTION_ID"); collectionStr != "" { // If collection ID provided.
		val, err := strconv.ParseInt(collectionStr, 10, 64) // Parse collection ID.
		if err != nil {                                     // If invalid.
			return AppConfig{}, fmt.Errorf("SHOPIFY_COLLECTION_ID must be an integer: %w", err) // Return error.
		} // End parse error.
		cfg.TargetCollectionID = val // Store collection ID.
	} // End collection parse.

	if cfg.ShopifyShopSubdomain == "" { // Ensure required config exists.
		return AppConfig{}, errors.New("missing SHOPIFY_SHOP (example: SHOPIFY_SHOP=myshop)") // Return missing env error.
	} // End required check.

	if cfg.ShopifyAdminToken == "" { // Ensure token exists.
		return AppConfig{}, errors.New("missing SHOPIFY_TOKEN (Shopify Admin API access token)") // Return missing env error.
	} // End required check.

	if cfg.SourceListingURL == "" { // Ensure source URL exists.
		return AppConfig{}, errors.New("missing SOURCE_URL (example: SOURCE_URL=https://www.ebay.com/itm/...)") // Return missing env error.
	} // End required check.

	return cfg, nil // Return valid config.
} // End loadConfigFromEnv.

func scrapeListing(ctx context.Context, cfg AppConfig) (Listing, error) { // Scrape listing from source URL.
	listing := Listing{} // Prepare empty listing container.

	collector := colly.NewCollector( // Create a new Colly collector.
		colly.AllowedDomains("www.ebay.com", "ebay.com"), // Restrict to eBay domains (avoid redirects to other hosts).
	) // End collector creation.

	collector.UserAgent = cfg.UserAgent // Set UA string for requests.

	collector.SetRequestTimeout(20 * time.Second) // Set request timeout per request.

	collector.Limit(&colly.LimitRule{ // Add polite rate limiting rules.
		DomainGlob:  "*ebay.com*",       // Apply to eBay domains.
		Parallelism: 1,                  // Single request at a time.
		Delay:       2 * time.Second,    // Delay between requests.
		RandomDelay: 1500 * time.Millisecond, // Random jitter to avoid looking like a hammer.
	}) // End limit rule.

	var scrapeErr error // Capture errors inside callbacks.

	collector.OnRequest(func(r *colly.Request) { // Called before each request.
		log.Printf("Scraping URL: %s\n", r.URL.String()) // Log the URL being scraped.
	}) // End OnRequest.

	collector.OnError(func(r *colly.Response, err error) { // Called when a request fails.
		scrapeErr = fmt.Errorf("scrape error: status=%d url=%s err=%w", r.StatusCode, r.Request.URL.String(), err) // Store error.
	}) // End OnError.

	collector.OnHTML("body", func(e *colly.HTMLElement) { // Called when body HTML is available.
		doc := e.DOM // Get goquery document.

		title := strings.TrimSpace(doc.Find("div.vim.x-item-title h1 span.ux-textspans--BOLD").Text()) // Try to locate title via selector.
		if title == "" {                                                                              // If title missing.
			title = strings.TrimSpace(doc.Find("h1 span").First().Text()) // Fallback: grab first h1 span text.
		} // End title fallback.
		listing.Title = title // Assign to listing.

		rawPrice := strings.TrimSpace(doc.Find("div.x-price-primary span.ux-textspans").First().Text()) // Extract displayed price text.
		if rawPrice == "" {                                                                              // If missing.
			rawPrice = strings.TrimSpace(doc.Find("[data-testid='x-price-primary'] span").First().Text()) // Fallback for alternative markup.
		} // End price fallback.

		parsedPrice, err := parseNumericPrice(rawPrice) // Parse numeric component from the raw price.
		if err != nil {                                 // If parsing fails.
			scrapeErr = fmt.Errorf("price parse failed: raw=%q err=%w", rawPrice, err) // Store error.
			return                                                                    // Stop processing.
		} // End parse error handling.

		listing.PriceUSD = parsedPrice + cfg.PriceMarkupUSD // Apply markup and store numeric price.

		desc := strings.TrimSpace(doc.Find("div.d-item-description p").First().Text()) // Attempt to extract first description paragraph.
		if desc == "" {                                                               // If missing.
			desc = strings.TrimSpace(doc.Find("#viTabs_0_is").Text()) // Fallback to older description/specs container.
		} // End desc fallback.
		listing.Description = desc // Assign description.

		imageSet := make(map[string]struct{}) // Use a set to deduplicate image URLs.

		doc.Find("div.ux-image-carousel-item.image-treatment img").Each(func(_ int, s *goquery.Selection) { // Iterate carousel images.
			url := strings.TrimSpace(s.AttrOr("data-zoom-src", "")) // Prefer zoom src.
			if url == "" {                                         // If zoom missing.
				url = strings.TrimSpace(s.AttrOr("src", "")) // Fallback to src.
			} // End url fallback.
			if url == "" { // If still empty.
				return // Skip.
			} // End empty check.
			imageSet[url] = struct{}{} // Add to set.
		}) // End image iteration.

		for url := range imageSet { // Convert set to slice.
			listing.ImageURLs = append(listing.ImageURLs, url) // Append each unique URL.
		} // End set conversion.
	}) // End OnHTML.

	done := make(chan error, 1) // Create a channel to signal completion.

	go func() { // Run visit in a goroutine so ctx can cancel.
		err := collector.Visit(cfg.SourceListingURL) // Start scraping.
		if err != nil {                              // If Visit fails.
			done <- err // Send error.
			return      // Exit goroutine.
		} // End visit error handling.
		collector.Wait()  // Wait for async callbacks.
		done <- scrapeErr // Send scrapeErr (nil if ok).
	}() // End goroutine.

	select { // Wait for ctx or scraping completion.
	case <-ctx.Done(): // If context canceled/timed out.
		return Listing{}, fmt.Errorf("scrape canceled: %w", ctx.Err()) // Return ctx error.
	case err := <-done: // If scraping completes.
		if err != nil { // If error exists.
			return Listing{}, err // Return scrape error.
		} // End error check.
	} // End select.

	if listing.Title == "" { // Validate required fields.
		return Listing{}, errors.New("scrape produced empty title (selectors may have changed)") // Return validation error.
	} // End title validation.

	if listing.PriceUSD <= 0 { // Validate price.
		return Listing{}, errors.New("scrape produced invalid price (<= 0)") // Return validation error.
	} // End price validation.

	return listing, nil // Return listing.
} // End scrapeListing.

func parseNumericPrice(raw string) (float64, error) { // Extract numeric price from raw string like "$12.34".
	raw = strings.TrimSpace(raw) // Trim whitespace.
	if raw == "" {               // If empty.
		return 0, errors.New("empty price string") // Return error.
	} // End empty check.

	re := regexp.MustCompile(`[\d.,]+`) // Regex to match numeric substring.
	numStr := re.FindString(raw)        // Extract first numeric occurrence.
	if numStr == "" {                   // If none found.
		return 0, fmt.Errorf("no numeric value found in %q", raw) // Return error.
	} // End numeric check.

	numStr = strings.ReplaceAll(numStr, ",", "") // Remove thousands separators.
	val, err := strconv.ParseFloat(numStr, 64)   // Parse as float.
	if err != nil {                              // If parsing fails.
		return 0, fmt.Errorf("failed to parse %q: %w", numStr, err) // Return error.
	} // End parse error.

	return val, nil // Return parsed price.
} // End parseNumericPrice.

func newShopifyClient(cfg AppConfig) *ShopifyClient { // Construct a Shopify client.
	return &ShopifyClient{ // Return pointer to client.
		httpClient: &http.Client{Timeout: 20 * time.Second}, // Create HTTP client with timeout.
		config:     cfg,                                     // Store config.
	} // End client.
} // End newShopifyClient.

func (sc *ShopifyClient) CreateProduct(ctx context.Context, listing Listing) (int64, error) { // Create a Shopify product.
	productPayload := ShopifyProductCreateRequest{ // Build create product payload.
		Product: ShopifyProduct{ // Populate product.
			Title:       listing.Title,              // Use scraped title.
			Vendor:      sc.config.ProductVendor,    // Use configured vendor.
			ProductType: sc.config.ProductType,      // Use configured product type.
			Tags:        sc.config.ProductTagsCSV,   // Use configured tags.
			Variants: []ShopifyVariant{ // Create a single-variant product.
				{Price: fmt.Sprintf("%.2f", listing.PriceUSD)}, // Format price to two decimals.
			}, // End variants.
			Images: buildShopifyImages(listing.ImageURLs), // Map image URLs to Shopify image payload.
		}, // End product.
	} // End payload.

	bodyBytes, err := json.Marshal(productPayload) // JSON encode payload.
	if err != nil {                                // If marshal fails.
		return 0, fmt.Errorf("marshal product payload failed: %w", err) // Return error.
	} // End marshal error.

	endpoint := fmt.Sprintf("https://%s.myshopify.com/admin/api/2025-10/products.json", sc.config.ShopifyShopSubdomain) // Build endpoint URL.

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes)) // Create request with ctx.
	if err != nil {                                                                            // If request creation fails.
		return 0, fmt.Errorf("create request failed: %w", err) // Return error.
	} // End request error.

	req.Header.Set("Content-Type", "application/json")                 // Set content type.
	req.Header.Set("X-Shopify-Access-Token", sc.config.ShopifyAdminToken) // Set Shopify token header.

	resp, err := sc.httpClient.Do(req) // Execute request.
	if err != nil {                    // If request fails.
		return 0, fmt.Errorf("product create HTTP failed: %w", err) // Return error.
	} // End http error.
	defer resp.Body.Close() // Ensure body is closed.

	respBody, err := io.ReadAll(resp.Body) // Read response body.
	if err != nil {                        // If read fails.
		return 0, fmt.Errorf("read create response failed: %w", err) // Return error.
	} // End read error.

	if resp.StatusCode < 200 || resp.StatusCode >= 300 { // Check for non-success status.
		return 0, fmt.Errorf("product create failed: status=%d body=%s", resp.StatusCode, string(respBody)) // Return error with body.
	} // End status check.

	var decoded ShopifyProductCreateResponse // Prepare decode target.
	if err := json.Unmarshal(respBody, &decoded); err != nil { // Decode JSON response.
		return 0, fmt.Errorf("decode create response failed: %w body=%s", err, string(respBody)) // Return error.
	} // End decode error.

	if decoded.Product.ID == 0 { // Validate product ID.
		return 0, fmt.Errorf("create response missing product ID: body=%s", string(respBody)) // Return error.
	} // End ID check.

	return decoded.Product.ID, nil // Return created product ID.
} // End CreateProduct.

func (sc *ShopifyClient) AddProductToCollection(ctx context.Context, productID int64, collectionID int64) error { // Add product to a collection.
	payload := ShopifyCollectCreateRequest{ // Build collect payload.
		Collect: ShopifyCollect{ // Fill collect.
			ProductID:    productID,    // Product ID.
			CollectionID: collectionID, // Collection ID.
		}, // End collect.
	} // End payload.

	bodyBytes, err := json.Marshal(payload) // Marshal to JSON.
	if err != nil {                         // If marshal fails.
		return fmt.Errorf("marshal collect payload failed: %w", err) // Return error.
	} // End marshal error.

	endpoint := fmt.Sprintf("https://%s.myshopify.com/admin/api/2025-10/collects.json", sc.config.ShopifyShopSubdomain) // Build endpoint.

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes)) // Create request.
	if err != nil {                                                                            // If request creation fails.
		return fmt.Errorf("create collect request failed: %w", err) // Return error.
	} // End request error.

	req.Header.Set("Content-Type", "application/json")                 // Set content type.
	req.Header.Set("X-Shopify-Access-Token", sc.config.ShopifyAdminToken) // Set token.

	resp, err := sc.httpClient.Do(req) // Execute request.
	if err != nil {                    // If request fails.
		return fmt.Errorf("collect create HTTP failed: %w", err) // Return error.
	} // End http error.
	defer resp.Body.Close() // Close response body.

	respBody, err := io.ReadAll(resp.Body) // Read response.
	if err != nil {                        // If read fails.
		return fmt.Errorf("read collect response failed: %w", err) // Return error.
	} // End read error.

	if resp.StatusCode < 200 || resp.StatusCode >= 300 { // Validate status.
		return fmt.Errorf("collect create failed: status=%d body=%s", resp.StatusCode, string(respBody)) // Return error.
	} // End status check.

	return nil // Success.
} // End AddProductToCollection.

func buildShopifyImages(urls []string) []ShopifyProductImage { // Convert URL strings to Shopify image payload.
	images := make([]ShopifyProductImage, 0, len(urls)) // Pre-allocate slice.
	for _, u := range urls {                            // Iterate URLs.
		u = strings.TrimSpace(u) // Trim whitespace.
		if u == "" {             // Skip empty.
			continue // Continue loop.
		} // End empty check.
		images = append(images, ShopifyProductImage{Src: u}) // Append image payload.
	} // End iteration.
	return images // Return result.
} // End buildShopifyImages.
