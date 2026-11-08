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

// The project the judge finished most recently (finished projects are appended to seen_projects)
export const lastJudgedProject = (judge: Judge) =>
    judge.seen_projects[judge.seen_projects.length - 1]?.project_id ?? null;

// The prizes (in settings order) the judge hasn't ranked their most recently finished project for
// yet: the ranked prizes for its HackUTD track, where it isn't in the judge's prize ranking. Judges
// must rank it for these before their next project (enforced by the judge page buttons only).
export const pendingPrizes = (judge: Judge, prizes: JudgePrize[]) => {
    const last = lastJudgedProject(judge);
    if (!last) return [];
    return prizes
        .filter((p) => p.project_ids.includes(last) && !p.ranking.includes(last))
        .map((p) => p.prize);
};

// Builds the URL of the prize ranking page that walks through the given prizes in order
export const prizeQueueUrl = (prizes: string[]) =>
    '/judge/prize?' + prizes.map((p) => `queue=${encodeURIComponent(p)}`).join('&');

/**
 * Ranking screen for one prize (a HackUTD track). The URL holds a queue of prizes (?queue=A&queue=B),
 * so after ranking a project the judge goes through each matching prize in order, then back to /judge.
 * The judge can't continue until the project they just finished is in this prize's ranking.
 */
const JudgePrizeRanking = () => {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const queue = searchParams.getAll('queue');
    const prizeName = queue[0] ?? '';
    const [judge, setJudge] = useState<Judge | null>(null);
    const [prize, setPrize] = useState<JudgePrize | null>(null);
    const [ranking, setRanking] = useState<string[]>([]);
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
            setRanking(found?.ranking ?? []);

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

    // The judge must drag the project they just finished into this prize's ranking before continuing
    // (not during deliberation, when rankings can't change)
    const last = lastJudgedProject(judge);
    const notRanked =
        !deliberation &&
        prize !== null &&
        last !== null &&
        prize.project_ids.includes(last) &&
        !ranking.includes(last);
    const lastSeen = judge.seen_projects.find((p) => p.project_id === last);
    const lastName = lastSeen
        ? `Table ${lastSeen.location}: ${lastSeen.name}`
        : 'your last project';
    const prizeTitle = prize?.prize ?? prizeName;

    return (
        <>
            <Helmet>
                <title>Prize Ranking | Jury</title>
            </Helmet>
            <JuryHeader withLogout />
            <Container noCenter className="px-2 pb-4">
                <Back location="/judge" />
                <h1 className="text-2xl mt-2">
                    Prize Ranking: <span className="font-bold text-prize">{prizeTitle}</span>
                </h1>
                <p className="text-light text-sm">
                    This is separate from your general ranking. Only the projects you&apos;ve seen
                    in this HackUTD track are shown. Rank them by how well they fit {prizeTitle}.
                </p>
                {notRanked && (
                    <p className="p-2 text-center mt-4 border-gold border-2 bg-gold/20 rounded-md">
                        Reminder: drag <b>{lastName}</b> into Ranked Projects for {prizeTitle} to
                        continue.
                    </p>
                )}
                <div className="w-full mb-2 mt-4">
                    {/* The 'error' style is the hot pink used for prize screens (grey while disabled) */}
                    <Button type="error" full disabled={notRanked} onClick={next}>
                        {nextLabel}
                    </Button>
                </div>
                {prize ? (
                    <Ranking
                        judge={judge}
                        deliberation={deliberation}
                        prize={prize}
                        onSave={setRanking}
                    />
                ) : (
                    <p className="text-light mt-4">There are no projects to rank for this prize.</p>
                )}
            </Container>
        </>
    );
};

export default JudgePrizeRanking;
