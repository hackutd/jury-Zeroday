---
sidebar_position: 4
title: Score Aggregation
description: How rankings are collected from judges and scores are aggregated.
---

# Score Aggregation

Jury's main feature revolves around its rank aggregation system. As judges view projects, they are asked to rank the projects they have seen. These rankings are aggregated to form a comprehensive ranking. In the following section, we go more in-depth on how this is done.

## Ranking

The basis of Jury's algorithm is the [Copeland Counting method](https://en.wikipedia.org/wiki/Copeland's_method). A judge will rank as many projects as they can (ideally as many as possible). See the [Judging Interface](/docs/usage/judge) page for a visual on the judges' UI.

The Copeland method works by breaking down rankings into pairwise comparisons. For example, if a judge has ranked projects in the order `A, B, C`, it would mean the same as the following comparisons: `(A, B), (B, C), (A, C)`, where we make `(winner, loser)` pairs. For every time a project is a "winner," it will get one point; every time it's a "loser," it will lose a point. These point values are then aggregated across all judges' rankings to form a final score for each project.

This method was chosen for its simplicity while supporting uneven counts of partial rankings between different judges.

## Final Deliberation

Once all projects are judged, organizers will go into a room to do final deliberation. They will use the scores from Jury to determine the final ranking of projects, as well as the rankings among any tracks. To help in the final decision, it's a good idea to send organizers out before final scores are tallied and view the top ranked projects on Jury, especially if there are multiple that stand out among the best.
