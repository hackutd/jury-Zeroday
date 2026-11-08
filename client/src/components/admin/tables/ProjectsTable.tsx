import { useEffect, useState } from 'react';
import ProjectRow from './ProjectRow';
import { useAdminStore, useAdminTableStore, useOptionsStore } from '../../../store';
import HeaderEntry from './HeaderEntry';
import { ProjectSortField } from '../../../enums';
import { getRequest } from '../../../api';
import { errorAlert, isFilterOnlyPrize } from '../../../util';
import AdminTable from './AdminTable';

const ProjectsTable = () => {
    const unsortedProjects = useAdminStore((state) => state.projects);
    const fetchProjects = useAdminStore((state) => state.fetchProjects);
    const [flags, setFlags] = useState<Flag[]>([]);
    const [sortState, setSortState] = useState<SortState<ProjectSortField>>({
        field: ProjectSortField.None,
        ascending: true,
    });
    const options = useOptionsStore((state) => state.options);
    const selectedTrack = useOptionsStore((state) => state.selectedTrack);
    const selectedPrize = useOptionsStore((state) => state.selectedPrize);
    const setSelected = useAdminTableStore((state) => state.setSelected);
    const projects = useAdminTableStore((state) => state.projects);
    const setProjects = useAdminTableStore((state) => state.setProjects);

    const updateSort = (field: SortField) => {
        if (sortState.field === field) {
            // If sorted on same field and descending, reset sort state
            if (!sortState.ascending) {
                setSortState({
                    field: ProjectSortField.None,
                    ascending: true,
                });
                setProjects(unsortedProjects);
                return;
            }

            // Otherwise, sort descending
            setSortState({
                field,
                ascending: false,
            });
        } else {
            // If in different sorted state, sort ascending on new field
            setSortState({
                field: field as ProjectSortField,
                ascending: true,
            });
        }
    };

    // On load, fetch projects
    useEffect(() => {
        fetchProjects();
    }, [fetchProjects]);

    // Fetch flags on load
    useEffect(() => {
        async function getFlags() {
            const res = await getRequest<Flag[]>('/admin/flags', 'admin');
            if (res.status !== 200) {
                errorAlert(res);
            }
            setFlags(res.data as Flag[]);
        }

        getFlags();
    }, []);

    // Best Beginner is only a filter with no prize score, so it shows the general score
    const showPrizeScore = selectedPrize !== '' && !isFilterOnlyPrize(selectedPrize);

    // When a prize is picked, sort by its score (general score for Best Beginner), highest first
    useEffect(() => {
        if (selectedPrize !== '') {
            setSortState({ field: ProjectSortField.Score, ascending: false });
        }
    }, [selectedPrize]);

    // When projects change, update projects and sort
    useEffect(() => {
        setSelected(Array(unsortedProjects.length).fill(false));

        sortAndFilterProjects();
    }, [sortState, selectedTrack, selectedPrize]);

    useEffect(() => {
        sortAndFilterProjects();
    }, [unsortedProjects]);

    const sortAndFilterProjects = () => {
        // Filter by track if enabled
        const filteredProjects =
            options.judge_tracks && selectedTrack !== ''
                ? unsortedProjects.filter((project) =>
                      project.challenge_list.some(
                          (c) => c.toLowerCase() === selectedTrack.toLowerCase()
                      )
                  )
                : unsortedProjects;

        // Then filter to the selected prize: ranked prizes match the project's HackUTD track (Devpost
        // "What Hack Utd Track" answer); Best Beginner shows the projects that answered "yes" to the
        // Devpost "Beginner Verification" question
        const prizeFilteredProjects =
            selectedPrize !== ''
                ? filteredProjects.filter((project) =>
                      isFilterOnlyPrize(selectedPrize)
                          ? project.beginner
                          : (project.hackutd_track ?? '').trim().toLowerCase() ===
                            selectedPrize.toLowerCase()
                  )
                : filteredProjects;

        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        let sortFunc = (_: Project, b: Project) => 0;
        const asc = sortState.ascending ? 1 : -1;
        switch (sortState.field) {
            case ProjectSortField.Name:
                sortFunc = (a, b) => a.name.localeCompare(b.name) * asc;
                break;
            case ProjectSortField.Flagged:
                sortFunc = (a, b) => {
                    const flagsA = flags
                        .filter((flag) => !flag.reason.includes('busy'))
                        .filter((flag) => flag.project_id === a.id).length;
                    const flagsB = flags
                        .filter((flag) => !flag.reason.includes('busy'))
                        .filter((flag) => flag.project_id === b.id).length;
                    return (flagsB - flagsA) * asc;
                };
                break;
            case ProjectSortField.TableNumber:
                sortFunc = (a, b) => (a.location - b.location) * asc;
                break;
            case ProjectSortField.Group:
                sortFunc = (a, b) => (a.group - b.group) * asc;
                break;
            case ProjectSortField.Score:
                sortFunc = (a, b) => {
                    if (showPrizeScore) {
                        const aps = a.prize_scores?.[selectedPrize] ?? 0;
                        const bps = b.prize_scores?.[selectedPrize] ?? 0;
                        return (aps - bps) * asc;
                    }
                    if (options.judge_tracks && selectedTrack !== '') {
                        const ats = a.track_scores?.[selectedTrack] ?? 0;
                        const bts = b.track_scores?.[selectedTrack] ?? 0;
                        return (ats - bts) * asc;
                    }
                    return (a.score - b.score) * asc;
                };
                break;
            case ProjectSortField.Seen:
                sortFunc = (a, b) => {
                    if (options.judge_tracks && selectedTrack !== '') {
                        const ats = a.track_seen[selectedTrack] ?? 0;
                        const bts = b.track_seen[selectedTrack] ?? 0;
                        return (ats - bts) * asc;
                    }
                    return (a.seen - b.seen) * asc;
                };
                break;
            case ProjectSortField.Updated:
                sortFunc = (a, b) => (a.last_activity - b.last_activity) * asc;
                break;
        }
        setProjects(prizeFilteredProjects.toSorted(sortFunc));
    };

    return (
        <AdminTable>
            <tr>
                <th className="w-12"></th>
                <HeaderEntry
                    name="Name"
                    updateSort={updateSort}
                    sortField={ProjectSortField.Name}
                    sortState={sortState}
                    align="left"
                />
                <HeaderEntry
                    name="Flagged"
                    updateSort={updateSort}
                    sortField={ProjectSortField.Flagged}
                    sortState={sortState}
                />
                <HeaderEntry
                    name="Table #"
                    updateSort={updateSort}
                    sortField={ProjectSortField.TableNumber}
                    sortState={sortState}
                />
                {options.multi_group && selectedTrack === '' && (
                    <HeaderEntry
                        name="Group"
                        updateSort={updateSort}
                        sortField={ProjectSortField.Group}
                        sortState={sortState}
                    />
                )}
                <HeaderEntry
                    name={showPrizeScore ? 'Prize Score' : 'Score'}
                    updateSort={updateSort}
                    sortField={ProjectSortField.Score}
                    sortState={sortState}
                />
                <HeaderEntry
                    name="Seen"
                    updateSort={updateSort}
                    sortField={ProjectSortField.Seen}
                    sortState={sortState}
                />
                <HeaderEntry
                    name="Updated"
                    updateSort={updateSort}
                    sortField={ProjectSortField.Updated}
                    sortState={sortState}
                />
                <th className="text-right w-24">Actions</th>
            </tr>
            {projects.map((project: Project, idx) => (
                <ProjectRow key={idx} idx={idx} project={project} />
            ))}
        </AdminTable>
    );
};

export default ProjectsTable;
