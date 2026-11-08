import { useEffect, useState } from 'react';
import Container from '../../components/Container';
import JuryHeader from '../../components/JuryHeader';
import { useNavigate } from 'react-router-dom';
import Button from '../../components/Button';
import StatBlock from '../../components/StatBlock';
import Loading from '../../components/Loading';
import { getRequest, postRequest } from '../../api';
import { errorAlert } from '../../util';
import Ranking from '../../components/judge/dnd/Ranking';
import { Helmet } from 'react-helmet';
import { lastJudgedProject, pendingPrizes, prizeQueueUrl } from './prize';

const Judge = () => {
    const navigate = useNavigate();
    const [prizes, setPrizes] = useState<JudgePrize[]>([]);
    const [judge, setJudge] = useState<Judge | null>(null);
    const [loaded, setLoaded] = useState(false);
    const [projCount, setProjCount] = useState(0);
    const [deliberation, setDeliberation] = useState(false);
    // The judge's saved general ranking, kept in sync as they drag (see Ranking onSave)
    const [ranking, setRanking] = useState<string[]>([]);
    // Show the general ranking for this visit if the project they just finished wasn't ranked yet
    // when the page loaded. It stays up after they drag it in, so they can still adjust its spot.
    const [showRanking, setShowRanking] = useState(false);

    // Verify user is logged in and read welcome before proceeding
    useEffect(() => {
        async function fetchData() {
            // Check to see if the user is logged in
            const loggedInRes = await postRequest<OkResponse>('/judge/auth', 'judge', null);
            if (loggedInRes.status === 401) {
                console.error(`Judge is not logged in!`);
                navigate('/judge/login');
                return;
            }
            if (loggedInRes.status !== 200) {
                errorAlert(loggedInRes);
                return;
            }
            if (loggedInRes.data?.ok !== 1) {
                console.error(`Judge is not logged in!`);
                navigate('/judge/login');
                return;
            }

            // Check for read welcome
            const readWelcomeRes = await getRequest<OkResponse>('/judge/welcome', 'judge');
            if (readWelcomeRes.status !== 200) {
                errorAlert(readWelcomeRes);
                return;
            }
            const readWelcome = readWelcomeRes.data?.ok === 1;
            if (!readWelcome) {
                navigate('/judge/welcome');
            }

            // Get the name & email of the user from the server
            const judgeRes = await getRequest<Judge>('/judge', 'judge');
            if (judgeRes.status !== 200) {
                errorAlert(judgeRes);
                return;
            }
            const judge: Judge = judgeRes.data as Judge;
            setJudge(judge);

            // Get the project count
            const projCountRes = await getRequest<ProjectCount>('/project/count', 'judge');
            if (projCountRes.status !== 200) {
                errorAlert(projCountRes);
                return;
            }
            setProjCount(projCountRes.data?.count as number);

            // Get deliberation status
            const delibRes = await getRequest<OkResponse>('/judge/deliberation', 'judge');
            if (delibRes.status !== 200) {
                errorAlert(delibRes);
                return;
            }
            const isDeliberation = delibRes.data?.ok === 1;
            setDeliberation(isDeliberation);

            // Seed the general ranking from the saved one, and decide whether to show it
            const savedRanking = judge.rankings ?? [];
            const last = lastJudgedProject(judge);
            setRanking(savedRanking);
            setShowRanking(!isDeliberation && last !== null && !savedRanking.includes(last));

            // Get the prizes the judge can rank for
            const prizesRes = await getRequest<JudgePrize[]>('/judge/prizes', 'judge');
            if (prizesRes.status !== 200) {
                errorAlert(prizesRes);
                return;
            }
            setPrizes(prizesRes.data as JudgePrize[]);

            setLoaded(true);
        }

        fetchData();
    }, []);

    if (!loaded || !judge) return <Loading disabled={!loaded || !judge} />;

    // After finishing a project, the judge must rank it for its HackUTD track (if Prize Rankings is on
    // for that track) before getting another project: Next Project is replaced by the prize ranking
    // button, and the reminder is shown on the prize screen. This comes from the saved rankings, so a
    // refresh or the Back link keeps it. Not during deliberation, when rankings are frozen.
    const prizeQueue = deliberation ? [] : pendingPrizes(judge, prizes);

    // The judge must rank the project they just finished in their general ranking first, then for its
    // prize (if any), before getting another project. Not during deliberation, when rankings are frozen.
    const last = lastJudgedProject(judge);
    const notRanked = !deliberation && last !== null && !ranking.includes(last);
    const lastSeen = judge.seen_projects.find((p) => p.project_id === last);
    const lastName = lastSeen
        ? `Table ${lastSeen.location}: ${lastSeen.name}`
        : 'your last project';

    return (
        <>
            <Helmet>
                <title>Judging | Jury</title>
            </Helmet>
            <JuryHeader withLogout />
            <Container noCenter className="px-2 pb-4">
                <h1 className="text-2xl mt-2">Welcome, {judge.name}!</h1>
                {judge.track !== '' && (
                    <p className="text-lighter font-bold">Track: {judge.track}</p>
                )}
                {notRanked && (
                    <p className="p-2 text-center mt-4 border-gold border-2 bg-gold/20 rounded-md">
                        Drag <b>{lastName}</b> into General Ranking to continue.
                    </p>
                )}
                <div className="w-full mb-6 mt-2">
                    {prizeQueue.length > 0 ? (
                        <Button
                            type="error"
                            full
                            onClick={() => navigate(prizeQueueUrl(prizeQueue))}
                            disabled={notRanked}
                        >
                            Next: Rank for {prizeQueue[0]}
                        </Button>
                    ) : (
                        <Button type="primary" full href="/judge/live" disabled={notRanked}>
                            {judge.current !== null ? 'Resume Project' : 'Next Project'}
                        </Button>
                    )}
                </div>
                <div className="flex justify-evenly">
                    <StatBlock name="Seen" value={judge.seen_projects.length + judge.flagged.length} />
                    <StatBlock name="Total Projects" value={projCount} />
                </div>
                {(showRanking || notRanked) && (
                    <Ranking judge={judge} deliberation={deliberation} onSave={setRanking} />
                )}
            </Container>
        </>
    );
};

export default Judge;
