import { useEffect, useState } from 'react';
import { getRequest } from '../../api';
import { errorAlert } from '../../util';

interface ChallengeBlockProps {
    /* Heading of the block (default "Challenge List") */
    title?: string;

    /* API path that returns the list of names (default the public /challenges) */
    path?: string;

    /* Auth to use for the request (default none) */
    auth?: '' | 'admin';
}

const ChallengeBlock = ({
    title = 'Challenge List',
    path = '/challenges',
    auth = '',
}: ChallengeBlockProps) => {
    const [challenges, setChallenges] = useState<string[]>([]);

    useEffect(() => {
        async function fetchData() {
            // Get challenges
            const challengesRes = await getRequest<string[]>(path, auth);
            if (challengesRes.status !== 200) {
                errorAlert(challengesRes);
                return;
            }
            setChallenges(challengesRes.data as string[]);
        }

        fetchData();
    }, [path, auth]);

    if (!challenges) return;

    return (
        <div className="border-2 border-primary bg-primary/10 rounded-md p-2 my-2">
            <h1 className="text-lg md:text-xl font-bold">{title}</h1>
            <p className="text-sm md:text-md text-light">{challenges.join(', ')}</p>
        </div>
    );
};

export default ChallengeBlock;
