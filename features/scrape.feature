Feature: Scraping a Prometheus endpoint
  As an operator
  I want counters rendered as rates over time
  So that the chart shows behaviour rather than an ever-climbing line

  Scenario: A counter becomes a rate
    Given an endpoint exposing counter "envoy_cluster_upstream_rq_total" at 1000
    When it is scraped
    And the endpoint reports 1300 and is scraped 15s later
    Then the series records 20 per second

  Scenario: A counter resets
    Given an endpoint exposing counter "envoy_cluster_upstream_rq_total" at 100
    When it is scraped
    And the endpoint reports 5 and is scraped 15s later
    Then the series records 0 per second

  Scenario: Malformed exposition does not stop the collector
    Given an endpoint returning a malformed body
    When it is scraped
    Then the scrape error count is 1
    And a later successful scrape still records the series

  Scenario: The target is unreachable
    Given an endpoint that refuses connections
    When it is scraped
    Then the scrape error count is 1
    And the collector is still running

  # The exposition splits a histogram across three suffixes; an operator thinks
  # of it as one metric, and so must the store. features/envoy.feature grades
  # the same criterion against Envoy's real output.
  Scenario: Histogram families group together
    Given an endpoint exposing histogram "envoy_cluster_upstream_rq_time" with 3 observations
    When it is scraped
    Then one family named "envoy_cluster_upstream_rq_time" is stored
    And no family is stored for its "_bucket", "_sum" or "_count" parts
    And the series records 3 observations
