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
import { pendingPrizes, prizeQueueUrl } from './prize';

const Judge = () => {
    const navigate = useNavigate();
    const [prizes, setPrizes] = useState<JudgePrize[]>([]);
    const [judge, setJudge] = useState<Judge | null>(null);
    const [loaded, setLoaded] = useState(false);
    const [projCount, setProjCount] = useState(0);
    const [deliberation, setDeliberation] = useState(false);

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
            setDeliberation(delibRes.data?.ok === 1);

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

    // After finishing a project, the judge must rank it for each prize it opted into before getting
    // the next project. This comes from the saved rankings, so a refresh or the Back link keeps it.
    const prizeQueue = pendingPrizes(judge, prizes);

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
                <div className="w-full mb-6 mt-2">
                    {prizeQueue.length > 0 ? (
                        <Button
                            type="primary"
                            full
                            onClick={() => navigate(prizeQueueUrl(prizeQueue))}
                        >
                            Next: Rank for {prizeQueue[0]}
                        </Button>
                    ) : (
                        <Button type="primary" full href="/judge/live">
                            {judge.current !== null ? 'Resume Project' : 'Next Project'}
                        </Button>
                    )}
                </div>
                <div className="flex justify-evenly">
                    <StatBlock name="Seen" value={judge.seen_projects.length + judge.flagged.length} />
                    <StatBlock name="Total Projects" value={projCount} />
                </div>
                <Ranking judge={judge} deliberation={deliberation} />
                {prizes.length > 0 && (
                    <>
                        <h2 className="text-primary text-xl font-bold mt-6">Prize Rankings</h2>
                        <p className="text-light text-sm">Edit your ranking for a prize.</p>
                        <div className="h-[1px] w-full bg-light my-2"></div>
                        <div className="flex flex-wrap gap-2">
                            {prizes.map((p) => (
                                <Button
                                    key={p.prize}
                                    type="outline-primary"
                                    className="text-lg"
                                    onClick={() => navigate(prizeQueueUrl([p.prize]))}
                                >
                                    {p.prize} ({p.project_ids.length})
                                </Button>
                            ))}
                        </div>
                    </>
                )}
            </Container>
        </>
    );
};

export default Judge;
