# listing-ingestion-pipeline

A structured ETL-style ingestion pipeline that demonstrates:

External HTML extraction

Data normalization and transformation

Deterministic price adjustment logic

Typed payload construction

API publication with explicit error handling

Controlled HTTP client behavior

This repository is intended as an architectural example of extracting semi-structured external data and publishing it into a controlled system via a typed API client.

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
