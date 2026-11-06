---
sidebar_position: 5
title: Tracks
description: How to judge tracks/challenges with Jury
---

# Judging Tracks (Challenges)

Tracks are generally smaller prizes within a hackathon that hackers can compete in, on top of competing for the general prize. To facilitate track judging, there are a couple of features within Jury that allow judges to give their opinions on tracks.

## Track Setup

On the admin settings page, there is a section dedicated to tracks:

![Track Settings](./assets/track-settings.png)

The first checkbox is used to enable/disable tracks. Disabling tracks will hide all track-related features, including adding judges by tracks and the track dropdown for the admin dashboard. With tracks disabled, Jury runs general judging only.

Once track judging is enabled, a **Keep General Judging** checkbox appears (checked by default):

| Enable Track Judging | Keep General Judging | Result |
|---|---|---|
| Off | (hidden) | General judging only |
| On | Checked | General and track judging run at the same time |
| On | Unchecked | Track judging only |

When general judging is turned off, the **General** option is removed from the admin dashboard dropdown, the add judge form and the QR codes page. New judges must be assigned a track (judges without a track are rejected, including CSV rows with an empty track), and any existing general judges will not be given projects.

When track judging is turned off, judges cannot be added with a track: the track field is hidden on the add judge form, CSV rows with a track are rejected, and track QR codes stop working when scanned. Judges who were already assigned a track keep it, and their rankings will **not** count toward the general scores, so make sure track judging is on if you have track judges.

:::warning
Make sure you do NOT toggle this on/off during judging as it may result in unexpected behavior when assigning/judging projects for track-specific judges!
:::

The main setting that will need to be set is the list of tracks to judge. Once all projects are uploaded into Jury, the set of all tracks will be displayed in the **Challenge List** section (will not display if no projects or no projects have any items in the `challenge_list` field). In the **List of Tracks** input box, please list all tracks that you would like to be judged by Jury. This will allow you to add judges that ONLY see projects for that track. Tracks are matched against the Challenge List **ignoring capitalization**, but the spelling (including spaces and punctuation) must match exactly. To ensure your tracks are correct, you can check on the admin dashboard to see if all the tracks you've listed contain projects.

You may also set the number of views each project gets per track. This is useful for smaller tracks to have projects viewed more times while larger tracks or less judges means you need to reduce the number of views per project to finish judging in time. Generally we recommend 2-4 views per project. Note that the track views list needs to be the same length as the track name list.

## Adding Track Judges

To add track-specific judges to Jury, you can use any of the three methods listed in the [Adding Judges](/docs/usage/admin/add-judges) page. For the CSV upload and form methods, simply fill in the track field with the track that you would like the judge to judge. The form will have a dropdown, but the CSV will need to have the track filled in with the **exact spelling** of the track (capitalization and surrounding spaces are ignored). Judges with a track that is not in the list of tracks are rejected.

To add a track judge by QR code, an [additional dropdown](/docs/usage/admin/add-judges#track-qr-code) will appear on the **Add Judges** page, which will let you view and generate QR codes for each track.

## Judging Tracks

Track judges judge the same way as general judges: they see projects one at a time, take notes, and **rank** the projects they have seen. Track judges only ever see projects in their own track, and each project is viewed at most the number of times set in the **Track Views** setting.

When picking a track judge's next project, Jury uses the same comparison-based approach as general judging, but only within the track:

1. Prioritized projects in the track are picked first
2. Only projects with the fewest views from that track's judges are considered
3. Of those, the projects that have been compared the least with what the judge has already seen are kept. This only counts comparisons made by **that track's judges**
4. Of those, the next project by table number is picked, so judges stay close to where they are

## Track Scores

Each track's score is calculated from the rankings of **that track's judges only**, using the same [scoring](/docs/usage/admin/scoring) as general judging. Selecting a track in the admin dashboard dropdown shows that track's scores and views. Track judges' rankings do **not** affect the general scores, and general judges do not affect track scores. Track judges should still get together after judging to decide on a winner, using the track scores.
