Feature: Streaming between instances
  As an operator
  I want to watch a target through another prism
  So that ten people looking at one endpoint do not scrape it ten times

  Background:
    Given a leader publishing a stream
    And a follower attached to it

  Scenario: A follower receives what the leader records
    When the leader records 3 series
    Then the follower holds 3 series

  Scenario: History survives a disconnect
    When the leader records 3 series
    And the leader goes away
    Then the follower still holds 3 series

  Scenario: The outage is reported rather than silent
    When the leader records 3 series
    And the leader goes away
    Then the follower reports reconnecting
    And the follower shows when it will retry
