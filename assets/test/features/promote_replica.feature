Feature: promote a replica
    When the master is down a replica should be promoted.
    Another replica should follow the new master.

  Background:
    Given SUT
      | pg_master   | master  |
      | pg_replica0 | replica |
      | pg_replica1 | replica |

    And the master is populated
    And replicas are synchronized

  Scenario: should promote a replica when the master is down
    When master is down
    Then a replica should be promoted
    And another replica should follow the new master
    And the new master should be populated with new data
    And replicas should contain new data
    When all up again
    Then replicas should contain new data
