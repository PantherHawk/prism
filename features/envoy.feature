@envoy
Feature: Envoy end to end
  As an operator pointing prism at the target it was built for
  I want Envoy's own exposition to render
  So that nothing on screen depends on prism having recognised Envoy

  # Every scenario reads the same bytes. Without PRISM_ENVOY_ENDPOINT they come
  # from features/testdata/envoy-stats.txt, recorded from the stack in deploy/;
  # with it set, they come from that admin port live. `make bdd-envoy` is the
  # live run, and it exists because a fixture is a moment in one Envoy version's
  # life and this is how we find out the moment has passed.
  Background:
    Given Envoy's stats endpoint

  Scenario: Every family Envoy declares is stored
    When Envoy is scraped
    Then every family the endpoint declared is held

  Scenario: A histogram is a single entry
    When Envoy is scraped
    Then no family is named after a histogram part
    And every histogram the endpoint declared is held once

  Scenario: Counters are charted as rates
    When Envoy is scraped
    And Envoy is scraped again 15s later
    Then "envoy_cluster_upstream_rq_total" is a counter
    And its charted value is a rate rather than the cumulative total

  Scenario: The clusters separate
    When Envoy is scraped
    Then pivoting "envoy_cluster_upstream_rq_total" on "envoy_cluster_name" gives one line per cluster

  Scenario: Envoy is not special-cased
    When Envoy is scraped
    And the same exposition is scraped with every "envoy_" prefix renamed
    Then both scrapes produce the same families, kinds and cardinalities
