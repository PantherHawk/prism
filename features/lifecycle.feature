Feature: Application lifecycle
  As an operator
  I want prism to start and stop predictably
  So that a failure never leaves a goroutine or a temporary file behind

  Scenario: No configuration file at the default path
    Given no configuration path is set
    When the application is built
    Then the build succeeds

  Scenario: Configuration is missing at an explicit path
    Given the configuration path is "/nonexistent/prism.yaml"
    When the application is built
    Then the build fails with "config file not found"
    And the returned application can still be cleaned up

  Scenario: A startup function fails
    Given a startup function that fails after 10ms
    And a startup function that blocks until cancelled
    When the application runs
    Then run returns that error
    And no startup goroutine is left blocked

  Scenario: Cleanup runs in reverse registration order
    Given cleanup functions "a", "b" and "c"
    When the application is cleaned up
    Then they ran in the order "c,b,a"

  Scenario: A cleanup function fails
    Given cleanup functions "a", "b" and "c"
    And cleanup function "b" fails
    When the application is cleaned up
    Then every cleanup function still ran
    And the cleanup error mentions "b failed"

  Scenario: The operator interrupts
    Given a startup function that blocks until cancelled
    When the context is cancelled while running
    Then run returns a cancellation error
