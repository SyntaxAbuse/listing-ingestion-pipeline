# listing-ingestion-pipeline

A structured ETL-style ingestion pipeline that demonstrates:

External HTML extraction

Data normalization and transformation

Deterministic price adjustment logic

Typed payload construction

API publication with explicit error handling

Controlled HTTP client behavior

This repository is intended as an architectural example of extracting semi-structured external data and publishing it into a controlled system via a typed API client.

# Limitations
This is a single-source ingestion example, not a distributed ingestion system.
No persistent queue or retry backoff strategy (by design — kept minimal).

# Example flow
External Listing
      ↓
HTML Extraction
      ↓
Structured Listing Object
      ↓
Normalized + Adjusted Price
      ↓
Typed Shopify Product Payload
      ↓
Create Product
      ↓
Add to Collection (optional)

# Overview

The pipeline follows a strict separation of concerns:

Extraction Layer

HTML parsing via Colly + goquery

Selector isolation

Fallback strategies

Controlled rate limiting

Normalization Layer

Currency parsing via regex

Numeric conversion with validation

Deterministic markup logic

Deduplicated image handling

Transformation Layer

Conversion to strongly-typed domain structs

Explicit API payload modeling

No map[string]interface{} ambiguity

Publication Layer

Typed Shopify Admin API client

Context-aware requests

Proper HTTP status validation

Strict JSON response decoding

Defensive error propagation

Architectural Intent

# This project is not a “scraper script.”

It is a demonstration of:

Responsible extraction patterns

Explicit system boundaries

Typed external API contracts

Deterministic transformation logic

Production-style error handling

The system intentionally avoids:

Inline secrets

Implicit type casting

Silent error swallowing

Unbounded request loops

Mixed responsibilities inside main()

# Configuration

All runtime configuration is handled via environment variables.

# Required:
SHOPIFY_SHOP
SHOPIFY_TOKEN
SOURCE_URL

# Optional
SHOPIFY_COLLECTION_ID
PRICE_MARKUP_USD
PRODUCT_VENDOR
PRODUCT_TYPE
PRODUCT_TAGS
USER_AGENT

# Design Decisions
Using explicit structs instead of generic maps:

Prevents runtime type ambiguity

Ensures compile-time validation

Makes the API contract explicit

Improves maintainability



