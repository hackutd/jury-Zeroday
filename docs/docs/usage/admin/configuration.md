---
sidebar_position: 1
title: Configuration
description: How to configure your Jury instance once set up
---

# Configuration and Settings

## Logging In

Once you have Jury up and running, make sure everything is working by logging into the admin portal. Use the admin password that you set in the environmental variables when setting up Jury. Once in the portal, you should see the admin dashboard shown below (though without any projects or judging progress):

![Admin Dashboard](./assets/dashboard.png)

Make sure where it shows “Test Hackathon Judging” on the picture is replaced by the name you set under `VITE_JURY_NAME` environmental variable.

On the dashboard, you should see a button at the top labeled **Settings**. This is where we will go to configure everything for Jury. Click it now.

## Settings

The settings page should look like the following:

![Admin Settings](./assets/admin-settings.png)

All settings will have a description describing their functionality, but we will go through each section one-by-one here for completeness. Note that a lot of the more destructive settings will be **disabled while judging is running**. This is to minimize the chance accidents happen and mess up judging.

### Judge Login

This section is mainly for after judging has started. Due to an experience with a brute-force login attack (due to the nature of judge codes), we have implemented a rate-limiting and login disabling system. The first button is used to disable logins. Once a judge is logged in, they can continue judging even if logins are disabled. We recommend **disabling logins once judging has started**.

Another option for mitigating brute force login attacks is to limit the number of logins per minute. This number can be changed at any time during judging, so another idea for preventing this sort of attack is to set the maximum to a low number such as 10 per minute. This allows for a few judges to log in without opening up the possibility for a brute force attack.

### Judging Parameters

Here, we define a couple of the parameters for the judging process. The first two buttons **reassign project numbers** and **reassign judge groups** are pretty self-explanatory. The former will remove table numbers from all projects and re-assign table numbers. This is generally done in the order that projects are added to Jury. Make sure you do NOT do this during judging as it will mess up the judging order! Generally, we use this if we've deleted a lot of projects in Jury and want to fill in the "holes" in tables (eg. you remove table 46 and want to shift all the other tables to fill the gap--this will obviously be bad during judging as it'll move all later tables). If you want to do this during judging (eg. add a new project to an empty table), you can use the "set table number" action on the [dashboard](/docs/usage/admin/dashboard). The **reassign judge groups** is a little complicated and will be explained on the [multi-group judging page](/docs/usage/admin/groups).

**Set Minimum Project Views** allows for you to specify the minimum number of views a project should get. The goal of Jury is to ensure that every project is compared to every other project approximately an equal number of times. However, we want to ensure all projects do get seen at LEAST a couple of times. Until all projects have been seen n times (set by this setting), the projects with the least number of views will be given to the next judge. This ensures that we are prioritizing all low-view projects until passing this threshold. Once past the threshold, the comparison-balancing algorithm will then be used to assign projects to judges.

**Ignore Tracks** is a feature primarily for batch uploading projects. If you have a track that you do NOT want judged with Jury, you can insert tracks that you want Jury to ignore here. When project(s) are uploaded to Jury, any project containing a track in the "Ignore Tracks" list will not be added to Jury. If you want all projects to be added to Jury, this is NOT where tracks to be judged are defined (see [below](#multi-group-and-track-judging)).

### Judging Clock and Timer

**Reset Main Clock** will reset the clock on the admin page--this is handy if you accidentally start the main clock and want to quickly reset it.

**Sync Clock with Database Automatically** is only useful if you have multiple instances of Jury or experience crashes. This will periodically store the main clock time in the database so that if Jury happens to restart the time won't be reset (as it's normally stored in memory).

**Backup Clock** will store the current clock value in the database.

**Set Judging Timer** determines how long each judge has to see a project. Judges will see a timer on their phone for each project, with its length determined by the value you set here. We find that 5 minutes is generally a good length and is defaulted to that.

### Multi-Group and Track Judging

In this section, you can toggle the multi-group judging and track judging options. Track judging allows judges to view only projects from specific tracks and rank them separately from the main judging. When track judging is on, you can also choose whether to keep general judging running alongside it. Multi-group judging can be used to group judges so they will only be assigned projects in their group, switching groups periodically to minimize walking. More detailed information about how to use track and group judging are on their respective pages:

- [Track Judging](/docs/usage/admin/tracks)
- [Multi-Group Judging](/docs/usage/admin/groups)

### Prize Rankings

Set the opt-in prizes that **every** judge also ranks for, as a comma-separated list (the Devpost challenge list is shown above the box for reference). Names are matched against each project's Devpost "Opt-In Prizes" ignoring case; opt-in prizes not listed here are not ranked.

After a judge finishes a project and ranks it as usual, the **Next** button takes them to one extra ranking screen for each listed prize that the project opted into, then back to their dashboard. On that screen they rank only the projects they've seen that opted into that prize. Judges can also edit any prize ranking later from the **Prize Rankings** buttons on their dashboard.

On the admin dashboard, a prize dropdown appears next to the toolbar buttons. Picking a prize shows only the projects that opted into it, sorted by their **Prize Score** (highest first). Prize scores use the same Copeland method as the general score, summed over every judge's ranking for that prize. Prize Rankings are separate from [track judging](/docs/usage/admin/tracks): they don't change which projects judges are sent to, and they don't affect the general score. **Clear Rankings** and **Clear Judging Data** also clear prize rankings.

**Best Beginner** is hardcoded as a filter only. It is always in the prize dropdown, and picking it shows the projects whose opt-in prizes contain "Best Beginner" (eg. "Best Beginner Hack"), sorted by their **general** score. Judges never get a ranking screen for it, and it is dropped if you add it to the Prize Rankings list.

### Export Data

This section allows you to export data from Jury. There are 6 exports, and a legend under the buttons shows what each one includes and whether it is **OK to share** or **Organizers only**:

- **Export Judges** - Exports all judges, including their login codes (organizers only)
- **Export Projects** - Exports all projects (organizers only)
- **Export Rankings** - Exports only the judges and their individual general rankings of projects, as table numbers (organizers only)
- **Export by Challenges** - Exports `prize-exports.zip` with two folders, each with one file per challenge (opt-in prize), named after the challenge (characters like `/` and `:` become `-`):
    - `csv-with-emails/` - the projects that submitted to that challenge, sorted by table, with Table, Name, Devpost and Try It Out columns followed by every team member's name and email (the same member columns as Export Participants). These contain personal info, so keep them with organizers.
    - `pdf/` - a printable list to hand to that prize's sponsor: the prize name at the top of every page, then each project's table number and name in large text, sorted by table, with page numbers. No personal info. Emoji and other symbols in names are left out, and letters outside Western European alphabets print as `?`.
- **Export Participants** - Exports `participants.csv` with one row per project, sorted by table: Table, Project and Devpost link, then every team member's name and email (the member columns are only filled in if projects were imported from the Devpost export _with_ personal info). Organizers only; use Export for Discord for a list to share with participants
- **Export for Discord** - Exports every project's name, table number and Devpost URL (in that column order), sorted alphabetically by project name (ignoring case), so hackers can look up their table in a list posted to Discord. It has no personal info, and includes hidden projects too

The CSV-only zip without emails is still available from the API at [`/admin/export/challenges`](/docs/reference/internal-api#get-adminexportchallenges).

### Reset Data

At the bottom of the settings page, there are multiple ways to reset data in Jury:

- **Clear Rankings**: This will remove all rankings that judges have made (including prize rankings) but keep all other info (judges, projects, settings).
- **Clear Judging Data**: On top of removing scores, this will also remove all views and judging progress, resetting Jury to right before judging started.
- **Delete all Projects**: This will delete all projects, judging data, and flags. It will not delete settings and judges.
- **Delete all Judges**: This will delete all judges, judging data, and flags. It will not delete settings and projects.
- **Drop Database**: This button will reset the database. It's pretty destructive, so obviously do not click it unless you are sure you want to COMPLETELY CLEAR Jury!!!
