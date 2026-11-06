import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Helmet } from 'react-helmet';
import Container from '../../components/Container';
import JuryHeader from '../../components/JuryHeader';
import Button from '../../components/Button';
import Back from '../../components/Back';
import Loading from '../../components/Loading';
import Ranking from '../../components/judge/dnd/Ranking';
import { getRequest, postRequest } from '../../api';
import { errorAlert } from '../../util';

// Builds the URL of the prize ranking page that walks through the given prizes in order
export const prizeQueueUrl = (prizes: string[]) =>
    '/judge/prize?' + prizes.map((p) => `queue=${encodeURIComponent(p)}`).join('&');

/**
 * Ranking screen for one opt-in prize. The URL holds a queue of prizes (?queue=A&queue=B), so after
 * ranking a project the judge goes through each matching prize in order, then back to /judge.
 */
const JudgePrizeRanking = () => {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const queue = searchParams.getAll('queue');
    const prizeName = queue[0] ?? '';
    const [judge, setJudge] = useState<Judge | null>(null);
    const [prize, setPrize] = useState<JudgePrize | null>(null);
    const [deliberation, setDeliberation] = useState(false);
    const [loaded, setLoaded] = useState(false);

    useEffect(() => {
        async function fetchData() {
            setLoaded(false);

            // Check to see if the user is logged in
            const loggedInRes = await postRequest<OkResponse>('/judge/auth', 'judge', null);
            if (
                loggedInRes.status === 401 ||
                (loggedInRes.status === 200 && loggedInRes.data?.ok !== 1)
            ) {
                console.error(`Judge is not logged in!`);
                navigate('/judge/login');
                return;
            }
            if (loggedInRes.status !== 200) {
                errorAlert(loggedInRes);
                return;
            }

            // Get the judge
            const judgeRes = await getRequest<Judge>('/judge', 'judge');
            if (judgeRes.status !== 200) {
                errorAlert(judgeRes);
                return;
            }
            setJudge(judgeRes.data as Judge);

            // Get the prizes the judge can rank for, and find this one
            const prizesRes = await getRequest<JudgePrize[]>('/judge/prizes', 'judge');
            if (prizesRes.status !== 200) {
                errorAlert(prizesRes);
                return;
            }
            const found = (prizesRes.data as JudgePrize[]).find(
                (p) => p.prize.toLowerCase() === prizeName.toLowerCase()
            );
            setPrize(found ?? null);

            // Get deliberation status
            const delibRes = await getRequest<OkResponse>('/judge/deliberation', 'judge');
            if (delibRes.status !== 200) {
                errorAlert(delibRes);
                return;
            }
            setDeliberation(delibRes.data?.ok === 1);

            setLoaded(true);
        }

        fetchData();
    }, [prizeName]);

    // Go to the next prize in the queue, or back to the judge page when done
    const next = () => {
        const rest = queue.slice(1);
        navigate(rest.length > 0 ? prizeQueueUrl(rest) : '/judge');
    };

    if (!loaded || !judge) return <Loading disabled={!loaded || !judge} />;

    const nextLabel = queue.length > 1 ? `Next: Rank for ${queue[1]}` : 'Done';

    return (
        <>
            <Helmet>
                <title>Prize Ranking | Jury</title>
            </Helmet>
            <JuryHeader withLogout />
            <Container noCenter className="px-2 pb-4">
                <Back location="/judge" />
                <h1 className="text-2xl mt-2">Rank for {prize?.prize ?? prizeName}</h1>
                <p className="text-light text-sm">
                    Only the projects you&apos;ve seen that opted into this prize are shown. Rank
                    them by how well they fit this prize.
                </p>
                <div className="w-full mb-2 mt-4">
                    <Button type="primary" full onClick={next}>
                        {nextLabel}
                    </Button>
                </div>
                {prize ? (
                    <Ranking judge={judge} deliberation={deliberation} prize={prize} />
                ) : (
                    <p className="text-light mt-4">There are no projects to rank for this prize.</p>
                )}
            </Container>
        </>
    );
};

export default JudgePrizeRanking;
