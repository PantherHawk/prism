Feature: Cardinality and sampling
  As an operator
  I want to know how many series a family has and whether I am seeing all of them
  So that a chart never quietly shows me a subset

  Scenario: A small family is counted exactly
    Given a family budget of 100
    When 40 series of "rq" are observed
    Then the reported cardinality is exactly 40
    And the family is not sampled
    And the cardinality is not an estimate

  Scenario: A family over its budget is sampled and says so
    Given a family budget of 100
    When 10000 series of "rq" are observed
    Then at most 100 series are stored
    And the family is sampled
    And the sampling ratio is a power of two
    And the reported cardinality is within 10% of 10000

  Scenario: Memory stays flat while cardinality climbs
    Given a family budget of 100
    When 10000 series of "rq" are observed
    And 100000 series of "rq" are observed
    Then at most 100 series are stored
    And the reported cardinality is within 10% of 100000

  Scenario: Sampling is reported for the whole snapshot
    Given a family budget of 100
    When 10000 series of "rq" are observed
    Then the snapshot reports a sampling ratio above 1

  Scenario: An unlimited family stores everything
    Given a family budget of 0
    When 500 series of "rq" are observed
    Then at most 500 series are stored
    And the family is not sampled
