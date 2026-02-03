// Canton Console Script: Archive old CIP56 contracts
// 
// Usage:
//   1. Connect to Canton console
//   2. Load this script: :load scripts/canton-archive-cip56.sc
//   3. Run dry-run first: dryRun()
//   4. If satisfied, run: archiveAll()
//
// This script archives CIP56Manager and CIP56Holding contracts created with
// the old package so new contracts can be created with the updated package.

import com.digitalasset.canton.console.ParticipantReference
import com.digitalasset.canton.protocol.LfContractId

// Configuration - update these values
val OLD_CIP56_PACKAGE_ID = "e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"
val ISSUER_PARTY = "daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"

// Templates to archive
val CIP56_MANAGER_TEMPLATE = s"$OLD_CIP56_PACKAGE_ID:CIP56.Token:CIP56Manager"
val CIP56_HOLDING_TEMPLATE = s"$OLD_CIP56_PACKAGE_ID:CIP56.Token:CIP56Holding"
val LOCKED_ASSET_TEMPLATE = s"$OLD_CIP56_PACKAGE_ID:CIP56.Token:LockedAsset"

// Dry run - list all contracts without archiving
def dryRun(): Unit = {
  println("=" * 70)
  println("DRY RUN - Listing CIP56 contracts (no changes will be made)")
  println("=" * 70)
  println(s"Old Package ID: $OLD_CIP56_PACKAGE_ID")
  println(s"Issuer Party: $ISSUER_PARTY")
  println()

  // Query CIP56Manager contracts
  println("CIP56Manager contracts:")
  println("-" * 50)
  val managers = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(CIP56_MANAGER_TEMPLATE)
  )
  if (managers.isEmpty) {
    println("  (none found)")
  } else {
    managers.foreach { contract =>
      println(s"  Contract ID: ${contract.event.contractId}")
      println(s"  Created at: ${contract.event.createdAt}")
      println()
    }
  }
  println(s"Total CIP56Manager: ${managers.size}")
  println()

  // Query CIP56Holding contracts
  println("CIP56Holding contracts:")
  println("-" * 50)
  val holdings = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(CIP56_HOLDING_TEMPLATE)
  )
  if (holdings.isEmpty) {
    println("  (none found)")
  } else {
    holdings.foreach { contract =>
      println(s"  Contract ID: ${contract.event.contractId}")
      println(s"  Created at: ${contract.event.createdAt}")
      println()
    }
  }
  println(s"Total CIP56Holding: ${holdings.size}")
  println()

  // Query LockedAsset contracts
  println("LockedAsset contracts:")
  println("-" * 50)
  val locked = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(LOCKED_ASSET_TEMPLATE)
  )
  if (locked.isEmpty) {
    println("  (none found)")
  } else {
    locked.foreach { contract =>
      println(s"  Contract ID: ${contract.event.contractId}")
      println(s"  Created at: ${contract.event.createdAt}")
      println()
    }
  }
  println(s"Total LockedAsset: ${locked.size}")
  println()

  println("=" * 70)
  println(s"SUMMARY: ${managers.size} managers, ${holdings.size} holdings, ${locked.size} locked")
  println("To archive all, run: archiveAll()")
  println("=" * 70)
}

// Archive all old CIP56 contracts
def archiveAll(): Unit = {
  println("=" * 70)
  println("ARCHIVING CIP56 contracts")
  println("=" * 70)

  var archived = 0
  var failed = 0

  // Archive CIP56Holding first (they reference the manager)
  println("Archiving CIP56Holding contracts...")
  val holdings = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(CIP56_HOLDING_TEMPLATE)
  )
  holdings.foreach { contract =>
    try {
      participant.ledger_api.commands.submit_and_wait(
        actAs = Seq(ISSUER_PARTY),
        commands = Seq(
          com.daml.ledger.api.v2.commands.Command().withArchive(
            com.daml.ledger.api.v2.commands.ArchiveCommand(
              templateId = Some(com.daml.ledger.api.v2.value.Identifier(
                packageId = OLD_CIP56_PACKAGE_ID,
                moduleName = "CIP56.Token",
                entityName = "CIP56Holding"
              )),
              contractId = contract.event.contractId
            )
          )
        )
      )
      println(s"  ✓ Archived CIP56Holding: ${contract.event.contractId.take(20)}...")
      archived += 1
    } catch {
      case e: Exception =>
        println(s"  ✗ Failed to archive CIP56Holding: ${e.getMessage}")
        failed += 1
    }
  }

  // Archive LockedAsset contracts
  println("Archiving LockedAsset contracts...")
  val locked = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(LOCKED_ASSET_TEMPLATE)
  )
  locked.foreach { contract =>
    try {
      participant.ledger_api.commands.submit_and_wait(
        actAs = Seq(ISSUER_PARTY),
        commands = Seq(
          com.daml.ledger.api.v2.commands.Command().withArchive(
            com.daml.ledger.api.v2.commands.ArchiveCommand(
              templateId = Some(com.daml.ledger.api.v2.value.Identifier(
                packageId = OLD_CIP56_PACKAGE_ID,
                moduleName = "CIP56.Token",
                entityName = "LockedAsset"
              )),
              contractId = contract.event.contractId
            )
          )
        )
      )
      println(s"  ✓ Archived LockedAsset: ${contract.event.contractId.take(20)}...")
      archived += 1
    } catch {
      case e: Exception =>
        println(s"  ✗ Failed to archive LockedAsset: ${e.getMessage}")
        failed += 1
    }
  }

  // Archive CIP56Manager last
  println("Archiving CIP56Manager contracts...")
  val managers = participant.ledger_api.acs.find_generic(
    filterParty = ISSUER_PARTY,
    filterTemplate = Some(CIP56_MANAGER_TEMPLATE)
  )
  managers.foreach { contract =>
    try {
      participant.ledger_api.commands.submit_and_wait(
        actAs = Seq(ISSUER_PARTY),
        commands = Seq(
          com.daml.ledger.api.v2.commands.Command().withArchive(
            com.daml.ledger.api.v2.commands.ArchiveCommand(
              templateId = Some(com.daml.ledger.api.v2.value.Identifier(
                packageId = OLD_CIP56_PACKAGE_ID,
                moduleName = "CIP56.Token",
                entityName = "CIP56Manager"
              )),
              contractId = contract.event.contractId
            )
          )
        )
      )
      println(s"  ✓ Archived CIP56Manager: ${contract.event.contractId.take(20)}...")
      archived += 1
    } catch {
      case e: Exception =>
        println(s"  ✗ Failed to archive CIP56Manager: ${e.getMessage}")
        failed += 1
    }
  }

  println()
  println("=" * 70)
  println(s"COMPLETE: Archived $archived contracts, $failed failures")
  println("=" * 70)
}

// Print usage on load
println()
println("CIP56 Archive Script loaded!")
println("  dryRun()    - List all contracts without archiving")
println("  archiveAll() - Archive all old CIP56 contracts")
println()
